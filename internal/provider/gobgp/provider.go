// Package gobgp implements the BGP provider interface for GoBGP.
//
// GoBGP operates as the overlay iBGP daemon in cosmos. It manages VPN sessions
// (IPv4/IPv6 VPNUnicast) between overlay nodes. It does NOT listen for inbound
// BGP connections (ListenPort == -1 / disabled) — all sessions are initiated
// outbound by GoBGP toward the remote peer's listening port.
//
// The implementation uses the GoBGP gRPC API (github.com/osrg/gobgp/v3/api)
// exclusively. There is no fallback; if the daemon is unreachable, all calls
// return an error.
//
// Thread safety: Provider is safe for concurrent use after New() returns.
// The underlying gRPC ClientConn is goroutine-safe per the gRPC documentation.
package gobgp

import (
	"context"
	"fmt"
	"strings"

	gobgpapi "github.com/osrg/gobgp/v3/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"go.miloapis.com/bgp/internal/provider"
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
	client   gobgpapi.GobgpApiClient
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
		client:   gobgpapi.NewGobgpApiClient(conn),
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
// Note: GoBGP's listen port is stored in SpeakerSpec.ListenPort. For the
// overlay role this is always -1 (listen disabled). The port value is passed
// through without enforcement here; the controller is responsible for setting
// it correctly per BGPInstance spec.
func (p *Provider) ConfigureSpeaker(ctx context.Context, spec provider.SpeakerSpec) error {
	// Probe current state.
	resp, err := p.client.GetBgp(ctx, &gobgpapi.GetBgpRequest{})
	if err != nil && grpcstatus.Code(err) != codes.NotFound {
		return fmt.Errorf("gobgp: ConfigureSpeaker GetBgp: %w", err)
	}

	needsRestart := resp == nil || resp.Global == nil
	if !needsRestart {
		g := resp.Global
		needsRestart = g.Asn != uint32(spec.ASNumber) ||
			g.RouterId != spec.RouterID ||
			g.ListenPort != spec.ListenPort
	}

	if !needsRestart {
		return nil
	}

	// Stop the current BGP instance if one is running.
	if resp != nil && resp.Global != nil {
		if _, err := p.client.StopBgp(ctx, &gobgpapi.StopBgpRequest{}); err != nil {
			return fmt.Errorf("gobgp: ConfigureSpeaker StopBgp: %w", err)
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
		return fmt.Errorf("gobgp: ConfigureSpeaker StartBgp AS=%d routerID=%s port=%d: %w",
			spec.ASNumber, spec.RouterID, spec.ListenPort, err)
	}
	return nil
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
		if err := p.applyDirectionalPolicy(ctx, spec.Name, spec.ImportStatements, gobgpapi.PolicyDirection_IMPORT); err != nil {
			return fmt.Errorf("gobgp: AddOrUpdatePolicy import %s: %w", spec.Name, err)
		}
	}
	if len(spec.ExportStatements) > 0 {
		if err := p.applyDirectionalPolicy(ctx, spec.Name, spec.ExportStatements, gobgpapi.PolicyDirection_EXPORT); err != nil {
			return fmt.Errorf("gobgp: AddOrUpdatePolicy export %s: %w", spec.Name, err)
		}
	}
	return nil
}

// DeletePolicy removes a route policy and all its assignments from GoBGP.
func (p *Provider) DeletePolicy(ctx context.Context, policyName string) error {
	// Remove all assignments first to avoid "policy in use" errors.
	if err := p.deleteAllAssignments(ctx, policyName); err != nil {
		// Log but do not abort — stale assignments are cleaned up best-effort.
		_ = err
	}

	_, err := p.client.DeletePolicy(ctx, &gobgpapi.DeletePolicyRequest{
		Policy:             &gobgpapi.Policy{Name: policyName},
		PreserveStatements: false,
		All:                false,
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("gobgp: DeletePolicy %s: %w", policyName, err)
	}
	return nil
}

// --- internal helpers ---------------------------------------------------------

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
				Family: &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_UNICAST},
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
	// Parse the CIDR to extract prefix and length.
	slash := strings.LastIndex(cidr, "/")
	if slash < 0 {
		return fmt.Errorf("invalid CIDR %q: missing prefix length", cidr)
	}
	prefix := cidr[:slash]
	var prefixLen uint32
	if _, err := fmt.Sscanf(cidr[slash+1:], "%d", &prefixLen); err != nil {
		return fmt.Errorf("parse prefix length from %q: %w", cidr, err)
	}

	nlri, err := anypb.New(&gobgpapi.IPAddressPrefix{
		PrefixLen: prefixLen,
		Prefix:    prefix,
	})
	if err != nil {
		return fmt.Errorf("marshal NLRI for %q: %w", cidr, err)
	}

	origin, err := anypb.New(&gobgpapi.OriginAttribute{Origin: 0}) // IGP
	if err != nil {
		return fmt.Errorf("marshal origin for %q: %w", cidr, err)
	}

	pattrs := []*anypb.Any{origin}
	family := &gobgpapi.Family{Afi: gobgpapi.Family_AFI_IP6, Safi: gobgpapi.Family_SAFI_UNICAST}

	_, err = p.client.AddPath(ctx, &gobgpapi.AddPathRequest{
		TableType: gobgpapi.TableType_GLOBAL,
		Path: &gobgpapi.Path{
			Family:     family,
			Nlri:       nlri,
			Pattrs:     pattrs,
			IsWithdraw: isWithdraw,
		},
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
	assignment := &gobgpapi.PolicyAssignment{
		Direction:     direction,
		DefaultAction: gobgpapi.RouteAction_ACCEPT,
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
				DefinedType: gobgpapi.DefinedType_PREFIX,
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
// An implicit final Accept statement is always appended to avoid implicit Reject.
func (p *Provider) upsertPolicy(ctx context.Context, name string, stmts []provider.PolicyStatement) error {
	// Build GoBGP Statements.
	gStmts := make([]*gobgpapi.Statement, 0, len(stmts)+1)
	for i, stmt := range stmts {
		gStmt := &gobgpapi.Statement{}

		if stmt.Conditions != nil && len(stmt.Conditions.PrefixSets) > 0 {
			gStmt.Conditions = &gobgpapi.Conditions{
				PrefixSet: &gobgpapi.MatchSet{
					Name: definedSetName(name, i),
					Type: gobgpapi.MatchSet_ANY,
				},
			}
		}

		gStmt.Actions = &gobgpapi.Actions{}
		switch stmt.Actions.RouteDisposition {
		case "Reject":
			gStmt.Actions.RouteAction = gobgpapi.RouteAction_REJECT
		default: // "Accept"
			gStmt.Actions.RouteAction = gobgpapi.RouteAction_ACCEPT
		}

		gStmts = append(gStmts, gStmt)
	}

	// Implicit final Accept — ensures routes not matched by earlier statements
	// are not silently dropped.
	gStmts = append(gStmts, &gobgpapi.Statement{
		Actions: &gobgpapi.Actions{RouteAction: gobgpapi.RouteAction_ACCEPT},
	})

	policy := &gobgpapi.Policy{Name: name, Statements: gStmts}

	_, err := p.client.AddPolicy(ctx, &gobgpapi.AddPolicyRequest{
		Policy:                  policy,
		ReferExistingStatements: false,
	})
	if err != nil {
		if !isAlreadyExists(err) {
			return fmt.Errorf("AddPolicy %s: %w", name, err)
		}
		// Replace: delete then re-add.
		if _, delErr := p.client.DeletePolicy(ctx, &gobgpapi.DeletePolicyRequest{
			Policy:             &gobgpapi.Policy{Name: name},
			PreserveStatements: false,
		}); delErr != nil && !isNotFound(delErr) {
			return fmt.Errorf("DeletePolicy %s for re-add: %w", name, delErr)
		}
		if _, addErr := p.client.AddPolicy(ctx, &gobgpapi.AddPolicyRequest{
			Policy:                  policy,
			ReferExistingStatements: false,
		}); addErr != nil {
			return fmt.Errorf("re-add policy %s: %w", name, addErr)
		}
	}
	return nil
}

// deleteAllAssignments removes every PolicyAssignment that references policyName.
// Best-effort — logs errors but does not surface them; the caller handles the
// final policy deletion.
func (p *Provider) deleteAllAssignments(ctx context.Context, policyName string) error {
	for _, dir := range []gobgpapi.PolicyDirection{gobgpapi.PolicyDirection_IMPORT, gobgpapi.PolicyDirection_EXPORT} {
		stream, err := p.client.ListPolicyAssignment(ctx, &gobgpapi.ListPolicyAssignmentRequest{
			Direction: dir,
		})
		if err != nil {
			continue
		}
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			if resp.Assignment == nil {
				continue
			}
			for _, pol := range resp.Assignment.Policies {
				if pol.Name != policyName {
					continue
				}
				_, _ = p.client.DeletePolicyAssignment(ctx, &gobgpapi.DeletePolicyAssignmentRequest{
					Assignment: resp.Assignment,
					All:        false,
				})
				break
			}
		}
	}
	return nil
}

// definedSetName returns the GoBGP DefinedSet name for a policy statement.
// Mirrors gobgpDefinedSetName from the legacy controller package.
func definedSetName(policyName string, statementIndex int) string {
	return fmt.Sprintf("%s-stmt%d", policyName, statementIndex)
}

// isAlreadyExists returns true for gRPC AlreadyExists errors and GoBGP's
// non-standard "can't overwrite" Unknown error.
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	code := grpcstatus.Code(err)
	msg := grpcstatus.Convert(err).Message()
	return code == codes.AlreadyExists ||
		(code == codes.Unknown && strings.Contains(msg, "can't overwrite the existing peer"))
}

// isNotFound returns true for gRPC NotFound errors.
func isNotFound(err error) bool {
	return err != nil && grpcstatus.Code(err) == codes.NotFound
}
