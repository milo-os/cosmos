package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPInstance configures BGP speaker parameters on all providers matching spec.providerSelector.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgpi
// +kubebuilder:printcolumn:name="AS",type="integer",JSONPath=".spec.asNumber"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPInstanceSpec   `json:"spec,omitempty"`
	Status BGPInstanceStatus `json:"status,omitempty"`
}

// BGPInstanceSpec defines the desired BGP speaker configuration.
//
// +kubebuilder:validation:XValidation:rule="self.routerIDSource != 'Manual' || has(self.routerID)",message="routerID is required when routerIDSource is Manual"
// +kubebuilder:validation:XValidation:rule="!has(self.routeReflector) || has(self.routeReflector.clusterID)",message="routeReflector.clusterID is required when routeReflector is set"
type BGPInstanceSpec struct {
	// ProviderSelector selects BGPProvider resources this instance configures.
	ProviderSelector metav1.LabelSelector `json:"providerSelector"`

	// ASNumber is the BGP autonomous system number.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	ASNumber int64 `json:"asNumber"`

	// RouterIDSource controls how the router ID is derived.
	// Auto derives from the last 32 bits of the backing node's IPv6 InternalIP.
	// Manual requires an explicit routerID field.
	//
	// +kubebuilder:default=Auto
	// +kubebuilder:validation:Enum=Auto;Manual
	// +optional
	RouterIDSource string `json:"routerIDSource,omitempty"`

	// RouterID is the explicit router ID. Required when routerIDSource is Manual.
	// BGP convention uses IPv4 dotted-quad format even on IPv6-only nodes.
	//
	// +kubebuilder:validation:Format=ipv4
	// +optional
	RouterID string `json:"routerID,omitempty"`

	// AddressFamilies lists the address families for this BGP instance.
	//
	// +kubebuilder:validation:MinItems=1
	AddressFamilies []AddressFamily `json:"addressFamilies"`

	// Timers contains BGP timer defaults for this instance.
	//
	// +optional
	Timers *BGPInstanceTimers `json:"timers,omitempty"`

	// BestPath controls BGP best-path selection parameters.
	//
	// +optional
	BestPath *BestPathConfig `json:"bestPath,omitempty"`

	// ListenPort is the TCP port the BGP speaker listens on for incoming sessions.
	// Defaults to 179 (standard BGP port). Set to -1 to disable the listener
	// entirely, which is appropriate for speakers that only initiate outbound sessions.
	//
	// +kubebuilder:default=179
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ListenPort *int32 `json:"listenPort,omitempty"`

	// RouteReflector configures this instance as a route reflector.
	// Only valid in infra cluster role. Rejected in POP clusters.
	//
	// +optional
	RouteReflector *RouteReflectorConfig `json:"routeReflector,omitempty"`
}

// BGPInstanceTimers holds default BGP timer values for the instance.
type BGPInstanceTimers struct {
	// DefaultHoldTime is the default hold time in seconds.
	//
	// +kubebuilder:default=90
	// +kubebuilder:validation:Minimum=3
	// +optional
	DefaultHoldTime int32 `json:"defaultHoldTime,omitempty"`

	// DefaultKeepalive is the default keepalive interval in seconds.
	//
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +optional
	DefaultKeepalive int32 `json:"defaultKeepalive,omitempty"`
}

// BestPathConfig controls BGP best-path selection behavior.
type BestPathConfig struct {
	// AlwaysCompareMed enables MED comparison across different ASes.
	// +optional
	AlwaysCompareMed bool `json:"alwaysCompareMed,omitempty"`

	// DeterministicMed enables deterministic MED comparison.
	// +optional
	DeterministicMed bool `json:"deterministicMed,omitempty"`

	// CompareRouterID uses router ID as a tiebreaker in best-path selection.
	// +optional
	CompareRouterID bool `json:"compareRouterId,omitempty"`
}

// RouteReflectorConfig enables route reflector operation.
type RouteReflectorConfig struct {
	// ClusterID is the route reflector cluster ID.
	// BGP convention uses IPv4 dotted-quad format.
	//
	// +kubebuilder:validation:Format=ipv4
	ClusterID string `json:"clusterID"`
}

// BGPInstanceStatus defines the observed state of BGPInstance.
type BGPInstanceStatus struct {
	// Conditions are top-level conditions for this BGPInstance.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Providers holds per-provider reconciliation status.
	//
	// +listType=map
	// +listMapKey=providerName
	// +optional
	Providers []ProviderStatus `json:"providers,omitempty"`
}

// +kubebuilder:object:root=true
type BGPInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPInstance{}, &BGPInstanceList{})
}
