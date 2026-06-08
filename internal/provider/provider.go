// Package provider defines the BGP provider interface for cosmos.
//
// EVOLUTION NOTE (Option B — out-of-process gRPC providers):
//
// This interface is intentionally designed to map 1:1 to a future gRPC service
// definition. When third-party or remote provider support is required:
//
//  1. Define a protobuf service from this interface. Each method becomes a
//     unary RPC. No streaming, no callbacks, no shared state between calls.
//
//  2. Each provider type (FRR, GoBGP, future types) becomes a separate process
//     exposing the gRPC service. The existing Go implementations become the
//     server-side logic.
//
//  3. The cosmos controller becomes a pure gRPC client. Remove the in-process
//     provider registry and replace with a dynamic client pool keyed by
//     BGPProvider.spec endpoint.
//
//  4. BGPProvider.spec gains credentialsSecretRef for mTLS. The endpoint field
//     already supports non-loopback addresses — remove the v1alpha1 restriction.
//
//  5. Capabilities() becomes a gRPC RPC called once after connection. Cache the
//     result in BGPProvider.status.capabilities.
//
// Do not add methods that cannot be expressed as stateless RPCs. Do not rely on
// in-process shared state between calls.
package provider

import (
	"context"
	"sync"
)

// Provider is the abstraction between the cosmos controller and a BGP daemon.
// All methods must be safe for concurrent use and must respect context cancellation.
//
// Implementations MUST be idempotent — calling any mutating method more than
// once with the same arguments must produce the same daemon state as calling it
// once. This contract enables the controller to safely re-apply configuration
// after a daemon restart without first querying existing state.
type Provider interface {
	// ConfigureSpeaker applies BGPInstance-level configuration. Idempotent.
	// On daemons that require a restart to change AS or router-ID (e.g. GoBGP
	// StartBgp/StopBgp), the implementation is responsible for detecting the
	// change and performing the restart transparently.
	// Returns (true, nil) when the daemon was restarted; peers must be re-applied
	// by the caller because a restart wipes all session state.
	ConfigureSpeaker(ctx context.Context, spec SpeakerSpec) (restarted bool, err error)

	// AddOrUpdatePeer configures a BGP session. Idempotent.
	AddOrUpdatePeer(ctx context.Context, peer PeerSpec) error

	// DeletePeer removes a BGP session. Idempotent — safe if peer does not exist.
	DeletePeer(ctx context.Context, address string) error

	// AddOrUpdateAdvertisement injects a prefix. Idempotent.
	AddOrUpdateAdvertisement(ctx context.Context, adv AdvertisementSpec) error

	// DeleteAdvertisement withdraws a prefix. Idempotent.
	DeleteAdvertisement(ctx context.Context, prefix string) error

	// AddOrUpdatePolicy applies a route policy. Idempotent.
	AddOrUpdatePolicy(ctx context.Context, policy PolicySpec) error

	// DeletePolicy removes a route policy. Idempotent.
	DeletePolicy(ctx context.Context, policyName string) error

	// Ready returns nil if the daemon is reachable and responsive.
	// Implementations should use a lightweight probe (e.g. GetBgp for GoBGP,
	// a gRPC health check for FRR) rather than establishing a new connection.
	Ready(ctx context.Context) error

	// Capabilities returns the provider's capability set.
	// In v1alpha1: compile-time constants per provider type.
	// Future: will become a cached gRPC call per the evolution note above.
	Capabilities(ctx context.Context) (CapabilitySet, error)
}

// AddressFamily represents a BGP address family (AFI/SAFI pair).
// String values match the kubebuilder enum in api/v1alpha1.
type AddressFamily struct {
	AFI  string // "IPv4" or "IPv6"
	SAFI string // "Unicast" or "VPNUnicast"
}

// SpeakerSpec is the provider-level representation of BGPInstance configuration.
// It is derived by the controller from a BGPInstance and its associated BGPProvider.
type SpeakerSpec struct {
	ASNumber int64
	RouterID string
	// ListenPort is 179 for FRR (standard BGP port). For GoBGP, it is 1790 on the
	// route reflector and -1 (listener disabled) on worker nodes, which only connect
	// outbound to the RR.
	ListenPort     int32
	Families       []AddressFamily
	Timers         TimerConfig
	BestPath       BestPathConfig
	RouteReflector *RouteReflectorConfig
}

// TimerConfig holds BGP hold-time and keepalive values.
type TimerConfig struct {
	HoldTime  int32
	Keepalive int32
}

// BestPathConfig controls BGP best-path selection behavior.
type BestPathConfig struct {
	AlwaysCompareMed bool
	DeterministicMed bool
	CompareRouterID  bool
}

// RouteReflectorConfig enables route reflector operation on the speaker.
type RouteReflectorConfig struct {
	// ClusterID is in IPv4 dotted-quad format (BGP convention).
	ClusterID string
}

// PeerSpec is the provider-level representation of BGPPeer configuration.
// It is derived by the controller from a BGPPeer, its BGPInstance, and optionally
// a resolved Secret for the password.
type PeerSpec struct {
	Address              string // IPv6
	ASNumber             int64
	Families             []AddressFamily
	Timers               TimerConfig
	AllowAsIn            int32
	RouteReflectorClient bool
	Passive              bool
	// EBGPMultihop is nil when not set. Mutually exclusive with TTLSecurity.
	EBGPMultihop *int32
	// TTLSecurity is nil when not set. Mutually exclusive with EBGPMultihop.
	TTLSecurity *int32
	// Password is the plaintext BGP session password. Empty string means no password.
	Password string
}

// AdvertisementSpec is the provider-level representation of BGPAdvertisement configuration.
type AdvertisementSpec struct {
	// Prefixes is the list of CIDR prefixes to inject into the RIB.
	Prefixes []string
	// PeerAddresses restricts advertisement to these peer addresses.
	// Empty slice means advertise to all peers.
	PeerAddresses []string
}

// PolicySpec is the provider-level representation of BGPRoutePolicy configuration.
type PolicySpec struct {
	Name             string
	Priority         int32
	ImportStatements []PolicyStatement
	ExportStatements []PolicyStatement
}

// PolicyStatement is one statement within a route policy.
type PolicyStatement struct {
	Name       string
	Conditions *PolicyConditions
	Actions    PolicyActions
}

// PolicyConditions holds the match conditions for a policy statement.
type PolicyConditions struct {
	// PrefixSets lists named prefix-set identifiers to match.
	PrefixSets []string
	// CommunitySet is the named community-set identifier to match. Empty means no match.
	CommunitySet string
	// NextHopSet is the named next-hop-set identifier to match. Empty means no match.
	NextHopSet string
}

// PolicyActions holds the actions taken when a statement matches.
type PolicyActions struct {
	// RouteDisposition is "Accept" or "Reject".
	RouteDisposition string
	// SetCommunity modifies the community attribute. Nil means no change.
	SetCommunity *SetCommunityAction
	// SetLocalPreference sets the LOCAL_PREF attribute. Nil means no change.
	SetLocalPreference *int32
	// SetMED sets the MED attribute. Nil means no change.
	SetMED *int32
	// SetNextHop overrides the next-hop. Empty string means no change.
	SetNextHop string
}

// SetCommunityAction describes how to modify the BGP community attribute.
type SetCommunityAction struct {
	// Communities is the list of community strings (e.g. "64512:100").
	Communities []string
	// Method is "Add", "Replace", or "Remove".
	Method string
}

// CapabilitySet describes the features supported by a provider implementation.
// In v1alpha1 these are compile-time constants; in future versions they will be
// queried from the daemon via a gRPC call.
type CapabilitySet struct {
	// AddressFamilies lists the AFI/SAFI combinations the daemon can advertise.
	AddressFamilies []AddressFamily
	// RouteReflection indicates whether the daemon supports RFC 4456 route reflection.
	RouteReflection bool
	// BFD indicates whether the daemon supports RFC 5880 bidirectional forwarding detection.
	BFD bool
}

// Registry maps BGPProvider resource names to their in-process Provider implementations.
// It is the control plane's live view of which daemons are currently configured.
//
// The registry is the v1alpha1 implementation of what will become a gRPC client pool
// keyed by BGPProvider endpoint in Option B (see package-level evolution note).
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Set registers or replaces the Provider for the given BGPProvider resource name.
func (r *Registry) Set(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// Get returns the Provider for the given BGPProvider resource name.
// The second return value is false when no provider is registered under name.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Delete removes the Provider registration for the given name.
// It is a no-op when name is not registered.
func (r *Registry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
}

// List returns a shallow copy of all registered providers keyed by name.
// The returned map is a snapshot — modifications to it do not affect the registry.
func (r *Registry) List() map[string]Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Provider, len(r.providers))
	for k, v := range r.providers {
		out[k] = v
	}
	return out
}
