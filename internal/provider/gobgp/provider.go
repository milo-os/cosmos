// Package gobgp implements the BGP provider interface for GoBGP.
//
// GoBGP operates as the overlay iBGP daemon in cosmos. It manages VPN sessions
// (IPv4/IPv6 VPNUnicast) between overlay nodes. The route reflector listens on
// port 1790 (non-standard, avoids conflicting with FRR on 179); worker nodes
// disable their listener entirely and connect outbound to the RR on port 1790.
//
// The implementation uses the GoBGP gRPC API (github.com/osrg/gobgp/v4/api)
// exclusively. There is no fallback; if the daemon is unreachable, all calls
// return an error.
//
// Thread safety: Provider is safe for concurrent use after New() returns.
// The underlying gRPC ClientConn is goroutine-safe per the gRPC documentation.
package gobgp

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	gobgpapi "github.com/osrg/gobgp/v4/api"
	"github.com/osrg/gobgp/v4/pkg/apiutil"
	bgppkt "github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"

	"go.miloapis.com/cosmos/internal/provider"
)

// GoBGPCapabilities are the compile-time capabilities of the GoBGP provider.
// GoBGP handles VPNUnicast families for the overlay; route reflection and BFD
// are not supported — those responsibilities belong to FRR in the underlay.
var GoBGPCapabilities = provider.CapabilitySet{
	AddressFamilies: []provider.AddressFamily{
		{AFI: "IPv4", SAFI: "VPNUnicast"},
		{AFI: "IPv6", SAFI: "VPNUnicast"},
	},
	RouteReflection: false,
	BFD:             false,
}

const defaultEndpoint = "localhost:50051"

// Provider implements provider.Provider for GoBGP.
type Provider struct {
	endpoint string
	conn     *grpc.ClientConn
	client   gobgpapi.GoBgpServiceClient
}

// New dials the GoBGP gRPC endpoint and returns a ready-to-use Provider.
// endpoint defaults to "localhost:50051" when empty.
// Returns an error if the gRPC connection cannot be established.
func New(endpoint string) (*Provider, error) {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("gobgp: dial %s: %w", endpoint, err)
	}
	return &Provider{
		endpoint: endpoint,
		conn:     conn,
		client:   gobgpapi.NewGoBgpServiceClient(conn),
	}, nil
}

// Close tears down the underlying gRPC connection. After Close the Provider
// must not be used.
func (p *Provider) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Ready returns nil when GoBGP is reachable. It performs a lightweight GetBgp
// probe — the same ping used by the legacy GoBGPClient.Connect() path.
func (p *Provider) Ready(ctx context.Context) error {
	_, err := p.client.GetBgp(ctx, &gobgpapi.GetBgpRequest{})
	if err != nil {
		// NotFound means GoBGP is running but BGP is not yet started. That's
		// reachable from a transport perspective — treat as ready so the
		// controller can proceed to call ConfigureSpeaker.
		if grpcstatus.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("gobgp: ready probe at %s: %w", p.endpoint, err)
	}
	return nil
}

// Capabilities returns the compile-time GoBGP capability set.
func (p *Provider) Capabilities(_ context.Context) (provider.CapabilitySet, error) {
	return GoBGPCapabilities, nil
}

// ConfigureSpeaker applies BGPInstance-level speaker configuration to GoBGP.
//
// GoBGP requires StopBgp/StartBgp to change the AS number, router-ID, or
// listen port. This method compares the running config against the desired
// spec and restarts only when a change is detected.
//
func (p *Provider) ConfigureSpeaker(ctx context.Context, spec provider.SpeakerSpec) (bool, error) {
	// Probe current state.
	resp, err := p.client.GetBgp(ctx, &gobgpapi.GetBgpRequest{})
	if err != nil && grpcstatus.Code(err) != codes.NotFound {
		return false, fmt.Errorf("gobgp: ConfigureSpeaker GetBgp: %w", err)
	}

	// GoBGP v4 returns Asn=0 (not NotFound) when the BGP engine has not been
	// started yet. Treat Asn==0 as uninitialised — do not call StopBgp, which
	// would terminate the gobgpd process.
	needsRestart := resp == nil || resp.Global == nil || resp.Global.Asn == 0
	if !needsRestart {
		g := resp.Global
		needsRestart = g.Asn != uint32(spec.ASNumber) ||
			g.RouterId != spec.RouterID ||
			g.ListenPort != spec.ListenPort
	}

	if !needsRestart {
		return false, nil
	}

	// Stop the current BGP instance if one is running.
	if resp != nil && resp.Global != nil && resp.Global.Asn != 0 {
		if _, err := p.client.StopBgp(ctx, &gobgpapi.StopBgpRequest{}); err != nil {
			return false, fmt.Errorf("gobgp: ConfigureSpeaker StopBgp: %w", err)
		}
	}

	_, err = p.client.StartBgp(ctx, &gobgpapi.StartBgpRequest{
		Global: &gobgpapi.Global{
			Asn:        uint32(spec.ASNumber),
			RouterId:   spec.RouterID,
			ListenPort: spec.ListenPort,
		},
	})
	if err != nil {
		return false, fmt.Errorf("gobgp: ConfigureSpeaker StartBgp AS=%d routerID=%s port=%d: %w",
			spec.ASNumber, spec.RouterID, spec.ListenPort, err)
	}
	return true, nil
}

// AddOrUpdatePeer configures a BGP session in GoBGP. It calls AddPeer on the
// first call for a given neighbor; subsequent calls with the same neighbor
// address call UpdatePeer instead.
//
// GoBGP returns codes.Unknown with the message "can't overwrite the existing peer"
// rather than codes.AlreadyExists — both cases are handled.
func (p *Provider) AddOrUpdatePeer(ctx context.Context, spec provider.PeerSpec) error {
	peer := buildPeer(spec)
	_, err := p.client.AddPeer(ctx, &gobgpapi.AddPeerRequest{Peer: peer})
	if err != nil {
		if isAlreadyExists(err) {
			if _, updateErr := p.client.UpdatePeer(ctx, &gobgpapi.UpdatePeerRequest{Peer: peer}); updateErr != nil {
				return fmt.Errorf("gobgp: UpdatePeer %s: %w", spec.Address, updateErr)
			}
			return nil
		}
		return fmt.Errorf("gobgp: AddPeer %s: %w", spec.Address, err)
	}
	return nil
}

// DeletePeer removes a BGP session from GoBGP. Safe to call when the peer
// does not exist — NotFound is treated as success.
func (p *Provider) DeletePeer(ctx context.Context, address string) error {
	_, err := p.client.DeletePeer(ctx, &gobgpapi.DeletePeerRequest{Address: address})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("gobgp: DeletePeer %s: %w", address, err)
	}
	return nil
}

// AddOrUpdateAdvertisement injects CIDR prefixes into GoBGP's global RIB.
//
// GoBGP's AddPath is inherently idempotent for the same NLRI + path attributes.
// The method advertises every prefix in spec.Prefixes using an IPv6 unicast path.
//
// Note: Advertisement scoping by peer address (spec.PeerAddresses) is not
// directly supported in the GoBGP AddPath API; route policy assignment is the
// GoBGP-native mechanism for per-peer advertisement control and is handled by
// AddOrUpdatePolicy.
func (p *Provider) AddOrUpdateAdvertisement(ctx context.Context, adv provider.AdvertisementSpec) error {
	for _, cidr := range adv.Prefixes {
		if err := p.addPath(ctx, cidr, false); err != nil {
			return fmt.Errorf("gobgp: AddOrUpdateAdvertisement prefix %s: %w", cidr, err)
		}
	}
	return nil
}

// DeleteAdvertisement withdraws a single CIDR prefix from GoBGP's global RIB.
func (p *Provider) DeleteAdvertisement(ctx context.Context, prefix string) error {
	if err := p.addPath(ctx, prefix, true); err != nil {
		return fmt.Errorf("gobgp: DeleteAdvertisement prefix %s: %w", prefix, err)
	}
	return nil
}

// AddOrUpdatePolicy applies a route policy to GoBGP.
//
// GoBGP's policy model has three layers: DefinedSets → Policies → PolicyAssignments.
// This method:
//  1. Creates or replaces PrefixSet DefinedSets for each statement condition.
//  2. Deletes and re-creates the Policy to handle statement changes cleanly.
//  3. Creates PolicyAssignments for both import and export directions.
//
// When spec.ImportStatements is non-empty the policy is assigned as an import
// policy; when spec.ExportStatements is non-empty as an export policy.
func (p *Provider) AddOrUpdatePolicy(ctx context.Context, spec provider.PolicySpec) error {
	// Build and apply DefinedSets + Policy for each direction.
	if len(spec.ImportStatements) > 0 {
		if err := p.applyDirectionalPolicy(ctx, spec.Name, spec.ImportStatements, gobgpapi.PolicyDirection_POLICY_DIRECTION_IMPORT); err != nil {
			return fmt.Errorf("gobgp: AddOrUpdatePolicy import %s: %w", spec.Name, err)
		}
	}
	if len(spec.ExportStatements) > 0 {
		if err := p.applyDirectionalPolicy(ctx, spec.Name, spec.ExportStatements, gobgpapi.PolicyDirection_POLICY_DIRECTION_EXPORT); err != nil {
			return fmt.Errorf("gobgp: AddOrUpdatePolicy export %s: %w", spec.Name, err)
		}
	}
	return nil
}

// DeletePolicy removes a route policy and all its assignments from GoBGP.
// Must remove assignments first so that DeletePolicy(All=true) is not blocked
// by GoBGP's inUse(activeIds) check.
func (p *Provider) DeletePolicy(ctx context.Context, policyName string) error {
	// Remove global assignments first (import and export).
	for _, dir := range []gobgpapi.PolicyDirection{gobgpapi.PolicyDirection_POLICY_DIRECTION_IMPORT, gobgpapi.PolicyDirection_POLICY_DIRECTION_EXPORT} {
		_, _ = p.client.DeletePolicyAssignment(ctx, &gobgpapi.DeletePolicyAssignmentRequest{
			Assignment: &gobgpapi.PolicyAssignment{
				Name:      "global",
				Direction: dir,
				Policies:  []*gobgpapi.Policy{{Name: policyName}},
			},
		})
	}

	// Delete the policy; All=true ensures inUse check considers activeIds but
	// since we removed assignments above, it passes. PreserveStatements=false
	// cleans up the auto-named inline statements.
	_, err := p.client.DeletePolicy(ctx, &gobgpapi.DeletePolicyRequest{
		Policy:             &gobgpapi.Policy{Name: policyName},
		PreserveStatements: false,
		All:                true,
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("gobgp: DeletePolicy %s: %w", policyName, err)
	}
	return nil
}

// --- internal helpers ---------------------------------------------------------

func remotePort(port int32) uint32 {
	if port == 0 {
		return 179
	}
	return uint32(port)
}

// buildPeer converts a PeerSpec into a gobgpapi.Peer.
func buildPeer(spec provider.PeerSpec) *gobgpapi.Peer {
	peer := &gobgpapi.Peer{
		Conf: &gobgpapi.PeerConf{
			NeighborAddress: spec.Address,
			PeerAsn:         uint32(spec.ASNumber),
		},
		Timers: &gobgpapi.Timers{
			Config: &gobgpapi.TimersConfig{
				HoldTime:          uint64(spec.Timers.HoldTime),
				KeepaliveInterval: uint64(spec.Timers.Keepalive),
			},
		},
		AfiSafis: buildAfiSafis(spec.Families),
		Transport: &gobgpapi.Transport{
			PassiveMode: spec.Passive,
			RemotePort:  remotePort(spec.RemotePort),
		},
	}

	if spec.AllowAsIn > 0 {
		peer.Conf.AllowOwnAsn = uint32(spec.AllowAsIn)
	}

	if spec.Password != "" {
		peer.Conf.AuthPassword = spec.Password
	}

	if spec.EBGPMultihop != nil {
		peer.EbgpMultihop = &gobgpapi.EbgpMultihop{
			Enabled:     true,
			MultihopTtl: uint32(*spec.EBGPMultihop),
		}
	}

	if spec.TTLSecurity != nil {
		peer.TtlSecurity = &gobgpapi.TtlSecurity{
			Enabled: true,
			TtlMin:  uint32(*spec.TTLSecurity),
		}
	}

	if spec.RouteReflectorClient {
		peer.RouteReflector = &gobgpapi.RouteReflector{
			RouteReflectorClient: true,
		}
	}

	return peer
}

// buildAfiSafis converts provider AddressFamily values to GoBGP AfiSafi structs.
// Defaults to IPv6 unicast when the list is empty.
func buildAfiSafis(families []provider.AddressFamily) []*gobgpapi.AfiSafi {
	if len(families) == 0 {
		return []*gobgpapi.AfiSafi{
			{Config: &gobgpapi.AfiSafiConfig{
				Family:  &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_UNICAST},
				Enabled: true,
			}},
		}
	}

	result := make([]*gobgpapi.AfiSafi, 0, len(families))
	for _, af := range families {
		afi, safi := afiSafiFromStrings(af.AFI, af.SAFI)
		result = append(result, &gobgpapi.AfiSafi{
			Config: &gobgpapi.AfiSafiConfig{
				Family:  &gobgpapi.Family{Afi: afi, Safi: safi},
				Enabled: true,
			},
		})
	}
	return result
}

// afiSafiFromStrings maps the AFI/SAFI enum strings to GoBGP constants.
func afiSafiFromStrings(afi, safi string) (gobgpapi.Family_Afi, gobgpapi.Family_Safi) {
	var a gobgpapi.Family_Afi
	switch afi {
	case "IPv4":
		a = gobgpapi.Family_AFI_IP
	default: // "IPv6"
		a = gobgpapi.Family_AFI_IP6
	}

	var s gobgpapi.Family_Safi
	switch safi {
	case "VPNUnicast":
		s = gobgpapi.Family_SAFI_MPLS_VPN
	default: // "Unicast"
		s = gobgpapi.Family_SAFI_UNICAST
	}
	return a, s
}

// addPath injects or withdraws a single IPv6 unicast CIDR in GoBGP's global RIB.
// This mirrors the addPathIPv6Prefix helper from the legacy controller package.
func (p *Provider) addPath(ctx context.Context, cidr string, isWithdraw bool) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	nlri, err := bgppkt.NewIPAddrPrefix(prefix.Masked())
	if err != nil {
		return fmt.Errorf("build NLRI for %q: %w", cidr, err)
	}

	// IPv6 locally-originated routes require MpReachNLRI with a next-hop.
	// "::" (unspecified) is the conventional self next-hop for locally-injected paths.
	mpReach, err := bgppkt.NewPathAttributeMpReachNLRI(
		bgppkt.RF_IPv6_UC,
		[]bgppkt.PathNLRI{{NLRI: nlri}},
		netip.MustParseAddr("::"),
	)
	if err != nil {
		return fmt.Errorf("build MpReachNLRI for %q: %w", cidr, err)
	}

	attrs := []bgppkt.PathAttributeInterface{
		bgppkt.NewPathAttributeOrigin(bgppkt.BGP_ORIGIN_ATTR_TYPE_IGP),
		mpReach,
	}

	path, err := apiutil.NewPath(bgppkt.RF_IPv6_UC, nlri, isWithdraw, attrs, time.Now())
	if err != nil {
		return fmt.Errorf("marshal path for %q: %w", cidr, err)
	}

	_, err = p.client.AddPath(ctx, &gobgpapi.AddPathRequest{
		TableType: gobgpapi.TableType_TABLE_TYPE_GLOBAL,
		Path:      path,
	})
	if err != nil {
		return fmt.Errorf("AddPath %q: %w", cidr, err)
	}
	return nil
}

// applyDirectionalPolicy creates/replaces DefinedSets, the Policy, and its
// PolicyAssignment for the given direction.
func (p *Provider) applyDirectionalPolicy(ctx context.Context, name string, stmts []provider.PolicyStatement, direction gobgpapi.PolicyDirection) error {
	// Step 1: upsert DefinedSets.
	if err := p.upsertDefinedSets(ctx, name, stmts); err != nil {
		return err
	}

	// Step 2: upsert the Policy itself.
	if err := p.upsertPolicy(ctx, name, stmts); err != nil {
		return err
	}

	// Step 3: upsert PolicyAssignment (global — no per-peer scoping at this level).
	// GoBGP requires Name="global" for the global routing table; empty string returns "empty table name".
	assignment := &gobgpapi.PolicyAssignment{
		Name:          "global",
		Direction:     direction,
		DefaultAction: gobgpapi.RouteAction_ROUTE_ACTION_ACCEPT,
		Policies:      []*gobgpapi.Policy{{Name: name}},
	}
	_, err := p.client.AddPolicyAssignment(ctx, &gobgpapi.AddPolicyAssignmentRequest{
		Assignment: assignment,
	})
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("AddPolicyAssignment %s dir=%v: %w", name, direction, err)
	}
	return nil
}

// upsertDefinedSets creates or replaces GoBGP PrefixSet DefinedSets for each
// statement that carries a non-empty PrefixSets condition.
func (p *Provider) upsertDefinedSets(ctx context.Context, policyName string, stmts []provider.PolicyStatement) error {
	for i, stmt := range stmts {
		if stmt.Conditions == nil || len(stmt.Conditions.PrefixSets) == 0 {
			continue
		}

		setName := definedSetName(policyName, i)
		// Build GoBGP Prefix entries from the string prefixes.
		prefixes := make([]*gobgpapi.Prefix, 0, len(stmt.Conditions.PrefixSets))
		for _, cidr := range stmt.Conditions.PrefixSets {
			prefixes = append(prefixes, &gobgpapi.Prefix{IpPrefix: cidr})
		}

		_, err := p.client.AddDefinedSet(ctx, &gobgpapi.AddDefinedSetRequest{
			DefinedSet: &gobgpapi.DefinedSet{
				DefinedType: gobgpapi.DefinedType_DEFINED_TYPE_PREFIX,
				Name:        setName,
				Prefixes:    prefixes,
			},
			Replace: true,
		})
		if err != nil {
			return fmt.Errorf("upsert defined set %s: %w", setName, err)
		}
	}
	return nil
}

// upsertPolicy creates or replaces a GoBGP Policy with the given statements.
//
// GoBGP's policy lifecycle requires a specific ordering to avoid "statement
// already defined" / "policy in use" errors across reconcile iterations:
//  1. Remove policy assignments (so DeletePolicy's inUse check passes).
//  2. Delete policy with All=true, PreserveStatements=false (deletes policy
//     AND cleans up auto-named statements, since statementInUse is now false).
//  3. Add the policy fresh with inline auto-named statements.
//
// An implicit final Accept statement is always appended to avoid implicit Reject.
func (p *Provider) upsertPolicy(ctx context.Context, name string, stmts []provider.PolicyStatement) error {
	// Build GoBGP Statements (no explicit Name — GoBGP will auto-name them "{policy}_stmt{i}").
	gStmts := make([]*gobgpapi.Statement, 0, len(stmts)+1)
	for i, stmt := range stmts {
		gStmt := &gobgpapi.Statement{}

		if stmt.Conditions != nil && len(stmt.Conditions.PrefixSets) > 0 {
			gStmt.Conditions = &gobgpapi.Conditions{
				PrefixSet: &gobgpapi.MatchSet{
					Name: definedSetName(name, i),
					Type: gobgpapi.MatchSet_TYPE_ANY,
				},
			}
		}

		gStmt.Actions = &gobgpapi.Actions{}
		switch stmt.Actions.RouteDisposition {
		case "Reject":
			gStmt.Actions.RouteAction = gobgpapi.RouteAction_ROUTE_ACTION_REJECT
		default: // "Accept"
			gStmt.Actions.RouteAction = gobgpapi.RouteAction_ROUTE_ACTION_ACCEPT
		}

		gStmts = append(gStmts, gStmt)
	}

	// Implicit final Accept — ensures routes not matched by earlier statements
	// are not silently dropped.
	gStmts = append(gStmts, &gobgpapi.Statement{
		Actions: &gobgpapi.Actions{RouteAction: gobgpapi.RouteAction_ROUTE_ACTION_ACCEPT},
	})

	// Step 1: Remove global assignments so the policy is no longer "in use".
	// GoBGP's DeletePolicy(All=true) blocks if inUse(activeIds) is true, which
	// it will be when the policy is assigned to the global routing table.
	for _, dir := range []gobgpapi.PolicyDirection{gobgpapi.PolicyDirection_POLICY_DIRECTION_IMPORT, gobgpapi.PolicyDirection_POLICY_DIRECTION_EXPORT} {
		_, _ = p.client.DeletePolicyAssignment(ctx, &gobgpapi.DeletePolicyAssignmentRequest{
			Assignment: &gobgpapi.PolicyAssignment{
				Name:      "global",
				Direction: dir,
				Policies:  []*gobgpapi.Policy{{Name: name}},
			},
		})
	}

	// Step 2: Delete the policy. With All=true, inUse is now false (assignments
	// removed), so deletion succeeds. PreserveStatements=false also removes the
	// auto-named statements from the statement map (since statementInUse returns
	// false after the policy is removed from policyMap).
	_, _ = p.client.DeletePolicy(ctx, &gobgpapi.DeletePolicyRequest{
		Policy:             &gobgpapi.Policy{Name: name},
		PreserveStatements: false,
		All:                true,
	})

	// Step 3: Create the policy fresh with new inline statements.
	policy := &gobgpapi.Policy{Name: name, Statements: gStmts}
	if _, err := p.client.AddPolicy(ctx, &gobgpapi.AddPolicyRequest{
		Policy:                  policy,
		ReferExistingStatements: false,
	}); err != nil {
		return fmt.Errorf("AddPolicy %s: %w", name, err)
	}
	return nil
}

// policyStatementName returns the explicit stable name for the i-th statement
// of a policy. Used by DeletePolicy cleanup to find and remove statements that
// outlived their policy.
func policyStatementName(policyName string, i int) string {
	return fmt.Sprintf("%s-s%d", policyName, i)
}

// definedSetName returns the GoBGP DefinedSet name for a policy statement.
// Mirrors gobgpDefinedSetName from the legacy controller package.
func definedSetName(policyName string, statementIndex int) string {
	return fmt.Sprintf("%s-stmt%d", policyName, statementIndex)
}

// isAlreadyExists returns true for gRPC AlreadyExists errors and GoBGP's
// non-standard Unknown errors that indicate a resource already exists.
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	code := grpcstatus.Code(err)
	if code == codes.AlreadyExists {
		return true
	}
	if code == codes.Unknown {
		msg := grpcstatus.Convert(err).Message()
		return strings.Contains(msg, "can't overwrite the existing peer") ||
			strings.Contains(msg, "already defined")
	}
	return false
}

// isNotFound returns true for gRPC NotFound errors, including GoBGP's
// non-standard codes.Unknown responses with "not found" in the message.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if grpcstatus.Code(err) == codes.NotFound {
		return true
	}
	// GoBGP returns codes.Unknown for missing resources (e.g. "not found policy: X").
	if grpcstatus.Code(err) == codes.Unknown {
		msg := strings.ToLower(grpcstatus.Convert(err).Message())
		return strings.Contains(msg, "not found")
	}
	return false
}
