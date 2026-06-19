package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AFI is the Address Family Indicator for a BGP address family.
//
// +kubebuilder:validation:Enum=ipv4;ipv6;l2vpn
type AFI string

const (
	AFIIPv4  AFI = "ipv4"
	AFIIPv6  AFI = "ipv6"
	AFIL2VPN AFI = "l2vpn"
)

// SAFI is the Subsequent Address Family Indicator for a BGP address family.
//
// +kubebuilder:validation:Enum=unicast;evpn
type SAFI string

const (
	SAFIUnicast SAFI = "unicast"
	SAFIEVPN    SAFI = "evpn"
)

// AddressFamily is a BGP address family expressed as an AFI/SAFI pair.
// Valid combinations: ipv4/unicast, ipv6/unicast, l2vpn/evpn.
//
// +kubebuilder:validation:XValidation:rule="self.afi == 'ipv4' ? self.safi == 'unicast' : true",message="IPv4 only supports unicast SAFI"
// +kubebuilder:validation:XValidation:rule="self.afi == 'ipv6' ? self.safi == 'unicast' : true",message="IPv6 only supports unicast SAFI"
// +kubebuilder:validation:XValidation:rule="self.afi == 'l2vpn' ? self.safi == 'evpn' : true",message="L2VPN only supports evpn SAFI"
type AddressFamily struct {
	// AFI is the address family indicator.
	AFI AFI `json:"afi"`

	// SAFI is the subsequent address family indicator.
	SAFI SAFI `json:"safi"`
}

// RouterRole defines the functional role of a BGPRouter within the network.
//
// +kubebuilder:validation:Enum=fabric;tenant;transit
type RouterRole string

const (
	RouterRoleFabric  RouterRole = "fabric"
	RouterRoleTenant  RouterRole = "tenant"
	RouterRoleTransit RouterRole = "transit"
)

// TargetRef identifies the execution target for a BGPRouter.
// Supported values for kind: Node.
type TargetRef struct {
	// Kind is the target resource kind (e.g. Node).
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the name of the target resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RouterRef is a direct reference to a single BGPRouter by name within the same namespace.
type RouterRef struct {
	// Name is the name of the BGPRouter.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RouterSelector selects one or more BGPRouter resources by label within the same namespace.
type RouterSelector struct {
	// MatchLabels is a map of key/value label pairs to match.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchExpressions is a list of label selector requirements.
	// +optional
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// RouterTarget is embedded by resources that bind to one or more BGPRouters.
// Exactly one of routerRef or routerSelector must be set.
//
// +kubebuilder:validation:XValidation:rule="has(self.routerRef) != has(self.routerSelector)",message="Exactly one of routerRef or routerSelector must be set"
type RouterTarget struct {
	// RouterRef targets a single BGPRouter by name.
	// Mutually exclusive with routerSelector.
	// +optional
	RouterRef *RouterRef `json:"routerRef,omitempty"`

	// RouterSelector targets one or more BGPRouters by label.
	// Mutually exclusive with routerRef.
	// +optional
	RouterSelector *RouterSelector `json:"routerSelector,omitempty"`
}

// LocalSecretRef references a Secret within the same namespace.
// Cross-namespace references are not supported.
type LocalSecretRef struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RouterStatus holds per-router reconciliation status used by BGPVRFInstance.
type RouterStatus struct {
	// RouterName is the name of the BGPRouter this entry describes.
	RouterName string `json:"routerName"`

	// Conditions are the per-router conditions.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResolvedConfig holds the configuration that was actually applied.
	//
	// +optional
	ResolvedConfig *ResolvedRouterConfig `json:"resolvedConfig,omitempty"`
}

// ResolvedRouterConfig holds the configuration resolved and applied to a specific router.
type ResolvedRouterConfig struct {
	// RouterID resolved for this router.
	// +optional
	RouterID string `json:"routerID,omitempty"`

	// ASNumber is the AS number configured.
	// +optional
	ASNumber *int64 `json:"asNumber,omitempty"`

	// AddressFamilies configured.
	// +optional
	AddressFamilies []AddressFamily `json:"addressFamilies,omitempty"`
}
