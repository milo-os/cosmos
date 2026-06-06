// Package routesync streams BGP RIB events from GoBGP and programs netlink routes.
package routesync

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	gobgpapi "github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/apiutil"
	"github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bgpnetlink "go.miloapis.com/cosmos/internal/netlink"
)

const routeWatchRetryInterval = 2 * time.Second

// RunRouteWatcher connects directly to a GoBGP gRPC endpoint, streams BGP path
// events, and programs/removes netlink routes (proto 196) for received prefixes.
// It automatically reconnects the event stream on error.
//
// endpoint is the GoBGP gRPC address (e.g. "localhost:50051").
// srv6Net is this node's own prefix (e.g. a /48); routes matching it are skipped
// so the node does not install a route to itself. Pass an empty string to disable
// the self-route filter.
//
// This function blocks until ctx is cancelled.
func RunRouteWatcher(ctx context.Context, endpoint, srv6Net string) {
	var ownPrefix *net.IPNet
	if srv6Net != "" {
		_, ownPrefix, _ = net.ParseCIDR(srv6Net)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c, conn, err := dialGoBGP(ctx, endpoint)
		if err != nil {
			log.Printf("bgp/route: connect to GoBGP at %s: %v — retrying in %s", endpoint, err, routeWatchRetryInterval)
			select {
			case <-ctx.Done():
				return
			case <-time.After(routeWatchRetryInterval):
			}
			continue
		}

		if err := watchAndProgram(ctx, c, ownPrefix); err != nil {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			default:
				log.Printf("bgp/route: stream error: %v — restarting in %s", err, routeWatchRetryInterval)
				conn.Close()
				select {
				case <-ctx.Done():
					return
				case <-time.After(routeWatchRetryInterval):
				}
			}
		}
	}
}

// dialGoBGP establishes a gRPC connection to the GoBGP endpoint.
func dialGoBGP(ctx context.Context, endpoint string) (gobgpapi.GoBgpServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}
	c := gobgpapi.NewGoBgpServiceClient(conn)
	// Ping to verify connectivity.
	if _, err := c.GetBgp(ctx, &gobgpapi.GetBgpRequest{}); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("ping: %w", err)
	}
	return c, conn, nil
}

// watchAndProgram opens a WatchEvent stream on GoBGP and programs netlink routes
// until the stream ends or ctx is cancelled.
//
// On startup, it loads the existing proto-196 routes from the kernel into
// knownPrefixes. After the initial RIB dump (paths with Init=true) arrives,
// any kernel route not present in the RIB is considered stale (left over from
// a previous operator lifetime) and is deleted.
func watchAndProgram(ctx context.Context, client gobgpapi.GoBgpServiceClient, ownPrefix *net.IPNet) error {
	// Seed knownPrefixes with all routes already in the kernel so stale routes
	// left from a prior operator run can be identified after the initial RIB dump.
	existingRoutes, err := bgpnetlink.ListManagedRoutes()
	if err != nil {
		log.Printf("bgp/route: list existing managed routes: %v", err)
	}
	knownPrefixes := make(map[string]net.IP, len(existingRoutes))
	for _, r := range existingRoutes {
		if r.Dst != nil {
			knownPrefixes[r.Dst.String()] = r.Gw
		}
	}
	if len(knownPrefixes) > 0 {
		log.Printf("bgp/route: loaded %d existing managed routes for stale-GC", len(knownPrefixes))
	}

	stream, err := client.WatchEvent(ctx, &gobgpapi.WatchEventRequest{
		Table: &gobgpapi.WatchEventRequest_Table{
			Filters: []*gobgpapi.WatchEventRequest_Table_Filter{
				{
					Type: gobgpapi.WatchEventRequest_Table_Filter_TYPE_BEST,
					Init: true,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("WatchEvent: %w", err)
	}

	ribInitPrefixes := make(map[string]struct{})
	initGCDone := false

	for {
		resp, err := stream.Recv()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("stream recv: %w", err)
			}
		}

		table := resp.GetTable()
		if table == nil {
			continue
		}

		for _, path := range table.Paths {
			if path.Family == nil ||
				path.Family.Afi != gobgpapi.Family_AFI_IP6 ||
				path.Family.Safi != gobgpapi.Family_SAFI_UNICAST {
				continue
			}

			prefix, nextHop, err := extractPrefixAndNextHop(path)
			if err != nil {
				log.Printf("bgp/route: skip path: %v", err)
				continue
			}

			// Skip our own prefix to avoid self-routing.
			if ownPrefix != nil && prefix.String() == ownPrefix.String() {
				continue
			}

			if !initGCDone {
				ribInitPrefixes[prefix.String()] = struct{}{}
			}

			if path.IsWithdraw {
				log.Printf("bgp/route: DEL route %s", prefix)
				if err := bgpnetlink.DelRoute(prefix); err != nil {
					log.Printf("bgp/route: del route %s: %v", prefix, err)
				}
				delete(knownPrefixes, prefix.String())
			} else {
				log.Printf("bgp/route: ADD route %s via %s", prefix, nextHop)
				if err := bgpnetlink.AddRoute(prefix, nextHop); err != nil {
					log.Printf("bgp/route: add route %s via %s: %v", prefix, nextHop, err)
				}
				knownPrefixes[prefix.String()] = nextHop
			}
		}

		if !initGCDone {
			initGCDone = true
			for prefix, gw := range knownPrefixes {
				if _, inRIB := ribInitPrefixes[prefix]; !inRIB {
					_, pfxNet, parseErr := net.ParseCIDR(prefix)
					if parseErr != nil {
						continue
					}
					log.Printf("bgp/route: GC stale route %s (gw=%s) — not in RIB after init", prefix, gw)
					if delErr := bgpnetlink.DelRoute(pfxNet); delErr != nil {
						log.Printf("bgp/route: GC del route %s: %v", prefix, delErr)
					}
					delete(knownPrefixes, prefix)
				}
			}
		}
	}
}

// extractPrefixAndNextHop parses the NLRI and next-hop from a GoBGP Path.
func extractPrefixAndNextHop(path *gobgpapi.Path) (*net.IPNet, net.IP, error) {
	nlri, err := apiutil.GetNativeNlri(path)
	if err != nil {
		return nil, nil, fmt.Errorf("get native NLRI: %w", err)
	}

	_, ipNet, err := net.ParseCIDR(nlri.String())
	if err != nil {
		return nil, nil, fmt.Errorf("parse prefix %s: %w", nlri.String(), err)
	}

	attrs, err := apiutil.GetNativePathAttributes(path)
	if err != nil {
		return nil, nil, fmt.Errorf("get native path attrs: %w", err)
	}

	var nextHop net.IP
	for _, attr := range attrs {
		switch a := attr.(type) {
		case *bgp.PathAttributeNextHop:
			nextHop = a.Value.AsSlice()
		case *bgp.PathAttributeMpReachNLRI:
			if a.Nexthop.IsValid() {
				nextHop = a.Nexthop.AsSlice()
			}
		}
	}
	if nextHop == nil {
		return nil, nil, fmt.Errorf("no next-hop in path attributes")
	}

	return ipNet, nextHop, nil
}
