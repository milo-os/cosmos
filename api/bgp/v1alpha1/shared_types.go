package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	AFIIPv4 = "IPv4"
	AFIIPv6 = "IPv6"

	SAFIUnicast    = "Unicast"
	SAFIVPNUnicast = "VPNUnicast"
)

// AddressFamily represents a BGP address family (AFI/SAFI pair).
type AddressFamily struct {
	// AFI is the Address Family Identifier.
	//
	// +kubebuilder:validation:Enum=IPv4;IPv6
	AFI string `json:"afi"`

	// SAFI is the Subsequent Address Family Identifier.
	//
	// +kubebuilder:validation:Enum=Unicast;VPNUnicast
	SAFI string `json:"safi"`
}

// ProviderStatus holds per-provider reconciliation status.
// Used in status.providers arrays across BGPInstance, BGPPeer, BGPAdvertisement, BGPRoutePolicy.
type ProviderStatus struct {
	// ProviderName is the name of the BGPProvider this entry describes.
	ProviderName string `json:"providerName"`

	// Daemon is the daemon type (FRR or GoBGP).
	Daemon string `json:"daemon"`

	// Conditions are the per-provider conditions.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResolvedConfig holds the configuration that was actually applied.
	//
	// +optional
	ResolvedConfig *ResolvedProviderConfig `json:"resolvedConfig,omitempty"`
}

// ResolvedProviderConfig holds the configuration resolved and applied to a specific provider.
// Fields are populated only when relevant for the resource type.
type ResolvedProviderConfig struct {
	// RouterID resolved for this provider (BGPInstance only).
	// +optional
	RouterID string `json:"routerID,omitempty"`

	// ListenPort is the port BGP listens on (179 for FRR, 0 for GoBGP which initiates only).
	// +optional
	ListenPort *int32 `json:"listenPort,omitempty"`

	// ASNumber is the AS number configured.
	// +optional
	ASNumber *int64 `json:"asNumber,omitempty"`

	// AddressFamilies configured.
	// +optional
	AddressFamilies []AddressFamily `json:"addressFamilies,omitempty"`

	// Timers resolved (merged from instance defaults and peer overrides).
	// +optional
	Timers *ResolvedTimers `json:"timers,omitempty"`

	// Address is the peer address (BGPPeer only).
	// +optional
	Address string `json:"address,omitempty"`

	// SessionType is iBGP or eBGP (BGPPeer only).
	// +optional
	SessionType string `json:"sessionType,omitempty"`

	// AllowAsIn is the allowas-in count (BGPPeer only).
	// +optional
	AllowAsIn *int32 `json:"allowAsIn,omitempty"`

	// Passive indicates passive mode (BGPPeer only).
	// +optional
	Passive *bool `json:"passive,omitempty"`

	// PasswordConfigured indicates whether a BGP session password was applied.
	// +optional
	PasswordConfigured *bool `json:"passwordConfigured,omitempty"`

	// ResolvedPrefixes is the list of prefixes injected (BGPAdvertisement only).
	// +optional
	ResolvedPrefixes []string `json:"resolvedPrefixes,omitempty"`
}

// ResolvedTimers holds effective BGP timer values after inheritance resolution.
type ResolvedTimers struct {
	HoldTime  int32 `json:"holdTime"`
	Keepalive int32 `json:"keepalive"`
}
