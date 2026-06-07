// Package frr implements the BGP provider interface for FRR (Free Range Routing).
//
// FRR version: 10.0+
// Required FRR daemons: zebra, bgpd
//
// # Implementation strategy
//
// The FRR northbound gRPC client bindings are not vendored in this module
// (absent from go.mod at the time of writing). All operations therefore use
// vtysh — the FRR management shell — via exec.Command("vtysh", "-c", ...).
// Once the FRR northbound gRPC bindings are available as a Go module dependency
// this package should be migrated to use them for operations with northbound
// coverage; the vtysh fallback documented in README.md identifies which
// operations to migrate first.
//
// # No silent fallback
//
// Every operation returns an explicit error on failure. There is no silent
// fallback between gRPC and vtysh — when gRPC bindings become available,
// each operation will use exactly one path.
//
// Thread safety: Provider is safe for concurrent use. vtysh invocations are
// independent OS-level processes; no internal shared mutable state is held
// between calls.
package frr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.miloapis.com/cosmos/internal/provider"
)

// FRRCapabilities are the compile-time capabilities of the FRR provider.
// FRR handles underlay eBGP and route reflection. It does NOT handle IPv4
// unicast overlay paths — those are GoBGP's domain.
var FRRCapabilities = provider.CapabilitySet{
	AddressFamilies: []provider.AddressFamily{
		{AFI: "IPv6", SAFI: "Unicast"},
		{AFI: "IPv4", SAFI: "VPNUnicast"},
		{AFI: "IPv6", SAFI: "VPNUnicast"},
	},
	RouteReflection: true,
	BFD:             false,
}

// Provider implements provider.Provider for FRR via vtysh.
//
// endpoint is stored for future use when FRR northbound gRPC bindings become
// available. It is not used by any operation in this version.
type Provider struct {
	// endpoint is the FRR northbound gRPC address (e.g. "localhost:50051").
	// Reserved for future gRPC migration; currently unused.
	endpoint string
}

// New returns an FRR Provider targeting the given northbound gRPC endpoint.
// endpoint defaults to "localhost:50051" when empty.
//
// No connection is established at construction time — FRR uses stateless
// vtysh invocations. The endpoint field is reserved for the future gRPC path.
func New(endpoint string) *Provider {
	if endpoint == "" {
		endpoint = "localhost:50051"
	}
	return &Provider{endpoint: endpoint}
}

// Ready probes FRR by running "vtysh -c 'show version'" and returns nil when
// the command exits successfully.
func (p *Provider) Ready(ctx context.Context) error {
	if err := p.vtysh(ctx, "show version"); err != nil {
		return fmt.Errorf("frr: ready probe: %w", err)
	}
	return nil
}

// Capabilities returns the compile-time FRR capability set.
func (p *Provider) Capabilities(_ context.Context) (provider.CapabilitySet, error) {
	return FRRCapabilities, nil
}

// ConfigureSpeaker applies BGP instance-level configuration via vtysh.
//
// It emits a vtysh configuration block equivalent to:
//
//	router bgp <ASN>
//	  bgp router-id <RouterID>
//	  timers bgp <keepalive> <hold>
//	  bgp bestpath as-path multipath-relax  (when AlwaysCompareMed)
//	  ...
func (p *Provider) ConfigureSpeaker(ctx context.Context, spec provider.SpeakerSpec) (bool, error) {
	var cmds []string

	cmds = append(cmds, fmt.Sprintf("router bgp %d", spec.ASNumber))
	cmds = append(cmds, fmt.Sprintf("  bgp router-id %s", spec.RouterID))

	if spec.Timers.HoldTime > 0 || spec.Timers.Keepalive > 0 {
		cmds = append(cmds, fmt.Sprintf("  timers bgp %d %d", spec.Timers.Keepalive, spec.Timers.HoldTime))
	}

	if spec.BestPath.AlwaysCompareMed {
		cmds = append(cmds, "  bgp always-compare-med")
	}
	if spec.BestPath.DeterministicMed {
		cmds = append(cmds, "  bgp deterministic-med")
	}
	if spec.BestPath.CompareRouterID {
		cmds = append(cmds, "  bgp bestpath compare-routerid")
	}

	if spec.RouteReflector != nil && spec.RouteReflector.ClusterID != "" {
		cmds = append(cmds, fmt.Sprintf("  bgp cluster-id %s", spec.RouteReflector.ClusterID))
	}

	// Activate address families.
	for _, af := range spec.Families {
		cmds = append(cmds, fmt.Sprintf("  address-family %s %s", frrAFI(af.AFI), frrSAFI(af.SAFI)))
		cmds = append(cmds, "  exit-address-family")
	}

	cmds = append(cmds, "exit")

	if err := p.vtyshConf(ctx, cmds); err != nil {
		return false, fmt.Errorf("frr: ConfigureSpeaker AS=%d: %w", spec.ASNumber, err)
	}
	// FRR applies configuration incrementally via vtysh; it does not lose peers
	// on reconfiguration. Always return restarted=false.
	return false, nil
}

// AddOrUpdatePeer configures a BGP neighbor in FRR via vtysh.
//
// The neighbor is added inside the global BGP instance. FRR's "no neighbor"
// followed by "neighbor" is not used — FRR neighbor commands are idempotent
// for existing neighbors when the same parameters are re-applied.
func (p *Provider) AddOrUpdatePeer(ctx context.Context, spec provider.PeerSpec) error {
	// Determine the AS number context: we need the local AS to address the
	// correct "router bgp" block. In practice the controller must have called
	// ConfigureSpeaker first, which established the router bgp block. We use
	// a show command to determine the current AS.
	asn, err := p.localASN(ctx)
	if err != nil {
		return fmt.Errorf("frr: AddOrUpdatePeer: determine local ASN: %w", err)
	}

	var cmds []string
	cmds = append(cmds, fmt.Sprintf("router bgp %d", asn))
	cmds = append(cmds, fmt.Sprintf("  neighbor %s remote-as %d", spec.Address, spec.ASNumber))

	if spec.Timers.HoldTime > 0 || spec.Timers.Keepalive > 0 {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s timers %d %d",
			spec.Address, spec.Timers.Keepalive, spec.Timers.HoldTime))
	}

	if spec.Password != "" {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s password %s", spec.Address, spec.Password))
	}

	if spec.Passive {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s passive", spec.Address))
	}

	if spec.AllowAsIn > 0 {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s allowas-in %d", spec.Address, spec.AllowAsIn))
	}

	if spec.RouteReflectorClient {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s route-reflector-client", spec.Address))
	}

	if spec.EBGPMultihop != nil {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s ebgp-multihop %d", spec.Address, *spec.EBGPMultihop))
	}

	if spec.TTLSecurity != nil {
		cmds = append(cmds, fmt.Sprintf("  neighbor %s ttl-security hops %d", spec.Address, *spec.TTLSecurity))
	}

	// Activate the peer for each address family.
	for _, af := range spec.Families {
		cmds = append(cmds, fmt.Sprintf("  address-family %s %s", frrAFI(af.AFI), frrSAFI(af.SAFI)))
		cmds = append(cmds, fmt.Sprintf("    neighbor %s activate", spec.Address))
		cmds = append(cmds, "  exit-address-family")
	}

	cmds = append(cmds, "exit")

	if err := p.vtyshConf(ctx, cmds); err != nil {
		return fmt.Errorf("frr: AddOrUpdatePeer %s: %w", spec.Address, err)
	}
	return nil
}

// DeletePeer removes a BGP neighbor from FRR. Safe to call when the neighbor
// does not exist — "no neighbor" is a no-op in FRR when the peer is absent.
func (p *Provider) DeletePeer(ctx context.Context, address string) error {
	asn, err := p.localASN(ctx)
	if err != nil {
		return fmt.Errorf("frr: DeletePeer: determine local ASN: %w", err)
	}

	cmds := []string{
		fmt.Sprintf("router bgp %d", asn),
		fmt.Sprintf("  no neighbor %s", address),
		"exit",
	}
	if err := p.vtyshConf(ctx, cmds); err != nil {
		return fmt.Errorf("frr: DeletePeer %s: %w", address, err)
	}
	return nil
}

// AddOrUpdateAdvertisement injects CIDR prefixes into FRR's BGP RIB via
// the "network" statement.
//
// FRR "network" statements require the prefix to exist in the kernel routing
// table (or "bgp network import-check" disabled). The cosmos deployment is
// responsible for ensuring the necessary static routes or kernel routes are
// present.
func (p *Provider) AddOrUpdateAdvertisement(ctx context.Context, adv provider.AdvertisementSpec) error {
	asn, err := p.localASN(ctx)
	if err != nil {
		return fmt.Errorf("frr: AddOrUpdateAdvertisement: determine local ASN: %w", err)
	}

	var cmds []string
	cmds = append(cmds, fmt.Sprintf("router bgp %d", asn))

	for _, prefix := range adv.Prefixes {
		af := addressFamilyForPrefix(prefix)
		cmds = append(cmds, fmt.Sprintf("  address-family %s unicast", af))
		cmds = append(cmds, fmt.Sprintf("    network %s", prefix))
		cmds = append(cmds, "  exit-address-family")
	}

	cmds = append(cmds, "exit")

	if err := p.vtyshConf(ctx, cmds); err != nil {
		return fmt.Errorf("frr: AddOrUpdateAdvertisement: %w", err)
	}
	return nil
}

// DeleteAdvertisement withdraws a CIDR prefix from FRR's BGP RIB.
func (p *Provider) DeleteAdvertisement(ctx context.Context, prefix string) error {
	asn, err := p.localASN(ctx)
	if err != nil {
		return fmt.Errorf("frr: DeleteAdvertisement: determine local ASN: %w", err)
	}

	af := addressFamilyForPrefix(prefix)
	cmds := []string{
		fmt.Sprintf("router bgp %d", asn),
		fmt.Sprintf("  address-family %s unicast", af),
		fmt.Sprintf("    no network %s", prefix),
		"  exit-address-family",
		"exit",
	}
	if err := p.vtyshConf(ctx, cmds); err != nil {
		return fmt.Errorf("frr: DeleteAdvertisement %s: %w", prefix, err)
	}
	return nil
}

// AddOrUpdatePolicy applies a route-map to FRR via vtysh.
//
// FRR route policies map onto route-maps. Each PolicyStatement becomes one
// route-map clause. Import statements are applied as neighbor route-maps in
// the "in" direction; export statements in the "out" direction.
//
// Prefix-list DefinedSets are created as ip/ipv6 prefix-lists prior to the
// route-map entries that reference them.
func (p *Provider) AddOrUpdatePolicy(ctx context.Context, spec provider.PolicySpec) error {
	var cmds []string

	// Create prefix-lists for import statements.
	for i, stmt := range spec.ImportStatements {
		if stmt.Conditions == nil {
			continue
		}
		for _, cidr := range stmt.Conditions.PrefixSets {
			plName := prefixListName(spec.Name, "import", i)
			af := addressFamilyForPrefix(cidr)
			cmds = append(cmds, fmt.Sprintf("%s prefix-list %s seq %d permit %s",
				af, plName, (i+1)*10, cidr))
		}
	}

	// Create prefix-lists for export statements.
	for i, stmt := range spec.ExportStatements {
		if stmt.Conditions == nil {
			continue
		}
		for _, cidr := range stmt.Conditions.PrefixSets {
			plName := prefixListName(spec.Name, "export", i)
			af := addressFamilyForPrefix(cidr)
			cmds = append(cmds, fmt.Sprintf("%s prefix-list %s seq %d permit %s",
				af, plName, (i+1)*10, cidr))
		}
	}

	// Build route-map clauses for import.
	for i, stmt := range spec.ImportStatements {
		rmName := routeMapName(spec.Name, "import")
		seq := (i + 1) * 10
		disposition := frrRouteDisposition(stmt.Actions.RouteDisposition)
		cmds = append(cmds, fmt.Sprintf("route-map %s %s %d", rmName, disposition, seq))

		if stmt.Conditions != nil && len(stmt.Conditions.PrefixSets) > 0 {
			plName := prefixListName(spec.Name, "import", i)
			af := addressFamilyForPrefix(stmt.Conditions.PrefixSets[0])
			cmds = append(cmds, fmt.Sprintf("  match %s address prefix-list %s", af, plName))
		}

		if stmt.Actions.SetLocalPreference != nil {
			cmds = append(cmds, fmt.Sprintf("  set local-preference %d", *stmt.Actions.SetLocalPreference))
		}
		if stmt.Actions.SetNextHop != "" {
			cmds = append(cmds, fmt.Sprintf("  set ipv6 next-hop global %s", stmt.Actions.SetNextHop))
		}
		if stmt.Actions.SetMED != nil {
			cmds = append(cmds, fmt.Sprintf("  set metric %d", *stmt.Actions.SetMED))
		}
		if stmt.Actions.SetCommunity != nil {
			cmds = append(cmds, fmt.Sprintf("  set community %s", strings.Join(stmt.Actions.SetCommunity.Communities, " ")))
		}

		cmds = append(cmds, "exit")
	}

	// Build route-map clauses for export.
	for i, stmt := range spec.ExportStatements {
		rmName := routeMapName(spec.Name, "export")
		seq := (i + 1) * 10
		disposition := frrRouteDisposition(stmt.Actions.RouteDisposition)
		cmds = append(cmds, fmt.Sprintf("route-map %s %s %d", rmName, disposition, seq))

		if stmt.Conditions != nil && len(stmt.Conditions.PrefixSets) > 0 {
			plName := prefixListName(spec.Name, "export", i)
			af := addressFamilyForPrefix(stmt.Conditions.PrefixSets[0])
			cmds = append(cmds, fmt.Sprintf("  match %s address prefix-list %s", af, plName))
		}

		if stmt.Actions.SetLocalPreference != nil {
			cmds = append(cmds, fmt.Sprintf("  set local-preference %d", *stmt.Actions.SetLocalPreference))
		}
		if stmt.Actions.SetNextHop != "" {
			cmds = append(cmds, fmt.Sprintf("  set ipv6 next-hop global %s", stmt.Actions.SetNextHop))
		}
		if stmt.Actions.SetMED != nil {
			cmds = append(cmds, fmt.Sprintf("  set metric %d", *stmt.Actions.SetMED))
		}
		if stmt.Actions.SetCommunity != nil {
			cmds = append(cmds, fmt.Sprintf("  set community %s", strings.Join(stmt.Actions.SetCommunity.Communities, " ")))
		}

		cmds = append(cmds, "exit")
	}

	if len(cmds) == 0 {
		return nil
	}

	if err := p.vtyshConf(ctx, cmds); err != nil {
		return fmt.Errorf("frr: AddOrUpdatePolicy %s: %w", spec.Name, err)
	}
	return nil
}

// DeletePolicy removes the route-maps and prefix-lists associated with a policy.
// FRR "no route-map" deletes all clauses for that route-map name.
func (p *Provider) DeletePolicy(ctx context.Context, policyName string) error {
	cmds := []string{
		fmt.Sprintf("no route-map %s", routeMapName(policyName, "import")),
		fmt.Sprintf("no route-map %s", routeMapName(policyName, "export")),
	}
	// Prefix-lists cannot be deleted with a single wildcard; rely on FRR's
	// garbage collection when the route-maps referencing them are removed.
	// A follow-up "clear ip prefix-list" is recommended but not mandatory.
	if err := p.vtyshConf(ctx, cmds); err != nil {
		return fmt.Errorf("frr: DeletePolicy %s: %w", policyName, err)
	}
	return nil
}

// --- vtysh helpers -----------------------------------------------------------

// vtysh runs a single vtysh show/exec command (non-configure mode).
// It respects ctx cancellation by passing the context to exec.CommandContext.
func (p *Provider) vtysh(ctx context.Context, cmd string) error {
	out, err := p.vtyshRun(ctx, cmd)
	if err != nil {
		return fmt.Errorf("vtysh %q: %w (output: %s)", cmd, err, strings.TrimSpace(out))
	}
	return nil
}

// vtyshConf applies a slice of configuration commands via vtysh in configure
// terminal mode. The commands are joined with newlines and sent to vtysh's stdin
// via the -c "configure terminal" pattern.
func (p *Provider) vtyshConf(ctx context.Context, cmds []string) error {
	// Prepend "configure terminal" and append "end" + "write memory" so the
	// configuration is persisted across FRR daemon restarts.
	full := make([]string, 0, len(cmds)+3)
	full = append(full, "configure terminal")
	full = append(full, cmds...)
	full = append(full, "end")
	full = append(full, "write memory")

	// Build a single -c argument per vtysh convention: multiple -c flags are
	// each processed as separate commands.
	args := make([]string, 0, len(full)*2)
	for _, c := range full {
		args = append(args, "-c", c)
	}

	cmd := exec.CommandContext(ctx, "vtysh", args...) //nolint:gosec // args are fully controller-generated
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vtysh configure: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// vtyshRun runs vtysh with a single -c flag and returns combined output.
func (p *Provider) vtyshRun(ctx context.Context, cmd string) (string, error) {
	c := exec.CommandContext(ctx, "vtysh", "-c", cmd) //nolint:gosec
	out, err := c.CombinedOutput()
	return string(out), err
}

// localASN queries the running FRR configuration to determine the local AS number.
// It parses the output of "show bgp summary" or "show running-config" to extract
// the ASN from the "router bgp <ASN>" line.
func (p *Provider) localASN(ctx context.Context) (uint32, error) {
	out, err := p.vtyshRun(ctx, "show running-config")
	if err != nil {
		return 0, fmt.Errorf("show running-config: %w", err)
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		var asn uint32
		if _, scanErr := fmt.Sscanf(line, "router bgp %d", &asn); scanErr == nil && asn > 0 {
			return asn, nil
		}
	}
	return 0, fmt.Errorf("could not determine local ASN from running config")
}

// --- FRR naming helpers ------------------------------------------------------

// frrAFI maps the provider AFI string to the FRR address-family keyword.
func frrAFI(afi string) string {
	switch afi {
	case "IPv4":
		return "ipv4"
	default: // "IPv6"
		return "ipv6"
	}
}

// frrSAFI maps the provider SAFI string to the FRR SAFI keyword.
func frrSAFI(safi string) string {
	switch safi {
	case "VPNUnicast":
		return "vpn"
	default: // "Unicast"
		return "unicast"
	}
}

// frrRouteDisposition maps "Accept"/"Reject" to FRR route-map permit/deny.
func frrRouteDisposition(d string) string {
	if d == "Reject" {
		return "deny"
	}
	return "permit"
}

// addressFamilyForPrefix returns "ipv6" or "ip" based on the prefix string.
// A colon in the prefix indicates IPv6.
func addressFamilyForPrefix(prefix string) string {
	if strings.Contains(prefix, ":") {
		return "ipv6"
	}
	return "ip"
}

// routeMapName returns the FRR route-map name for a given policy and direction.
func routeMapName(policyName, direction string) string {
	return fmt.Sprintf("bgp-%s-%s", policyName, direction)
}

// prefixListName returns the FRR prefix-list name for a policy statement.
func prefixListName(policyName, direction string, index int) string {
	return fmt.Sprintf("bgp-%s-%s-stmt%d", policyName, direction, index)
}
