package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BGPProvider represents one instance of a BGP daemon process that cosmos can configure.
// It carries the daemon type, endpoint, capabilities, and node binding via labels.
// Auto-bootstrapped at controller startup for local daemons.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgpp
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPProviderSpec   `json:"spec,omitempty"`
	Status BGPProviderStatus `json:"status,omitempty"`
}

// BGPProviderSpec defines the desired state of BGPProvider.
//
// +kubebuilder:validation:XValidation:rule="has(self.frr) != has(self.gobgp)",message="exactly one of frr or gobgp must be set"
// +kubebuilder:validation:XValidation:rule="self.type == 'FRR' ? has(self.frr) : has(self.gobgp)",message="type must match the set daemon block"
type BGPProviderSpec struct {
	// Type is the BGP daemon type. Must match the set daemon block.
	//
	// +kubebuilder:validation:Enum=FRR;GoBGP
	Type string `json:"type"`

	// FRR contains FRR northbound gRPC configuration.
	// Required when type is FRR.
	//
	// +optional
	FRR *FRRProviderConfig `json:"frr,omitempty"`

	// GoBGP contains GoBGP gRPC configuration.
	// Required when type is GoBGP.
	//
	// +optional
	GoBGP *GoBGPProviderConfig `json:"gobgp,omitempty"`
}

// FRRProviderConfig holds FRR-specific endpoint configuration.
type FRRProviderConfig struct {
	// Endpoint is the gRPC endpoint for the FRR northbound API.
	// In v1alpha1 only loopback addresses are accepted.
	//
	// +kubebuilder:default="localhost:50051"
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// GoBGPProviderConfig holds GoBGP-specific endpoint configuration.
type GoBGPProviderConfig struct {
	// Endpoint is the gRPC endpoint for the GoBGP API.
	// In v1alpha1 only loopback addresses are accepted.
	//
	// +kubebuilder:default="localhost:50051"
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// BGPProviderStatus defines the observed state of BGPProvider.
type BGPProviderStatus struct {
	// Conditions reflect the current state of the BGPProvider.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Capabilities is the self-reported capability set.
	// Populated by cosmos at reconcile time from compile-time constants in v1alpha1.
	//
	// +optional
	Capabilities *ProviderCapabilities `json:"capabilities,omitempty"`

	// ResolvedEndpoint is the endpoint currently in use.
	//
	// +optional
	ResolvedEndpoint string `json:"resolvedEndpoint,omitempty"`

	// Daemon is the daemon type of this provider.
	//
	// +optional
	Daemon string `json:"daemon,omitempty"`
}

// ProviderCapabilities describes what a BGP daemon can do.
type ProviderCapabilities struct {
	// AddressFamilies lists the AFI/SAFI combinations supported.
	//
	// +optional
	AddressFamilies []AddressFamilyCapability `json:"addressFamilies,omitempty"`

	// RouteReflection indicates whether the daemon supports route reflector operation.
	RouteReflection bool `json:"routeReflection"`

	// BFD indicates whether the daemon supports BFD.
	BFD bool `json:"bfd"`
}

// AddressFamilyCapability represents a supported AFI/SAFI combination.
type AddressFamilyCapability struct {
	// AFI is the Address Family Identifier.
	//
	// +kubebuilder:validation:Enum=IPv4;IPv6
	AFI string `json:"afi"`

	// SAFI is the Subsequent Address Family Identifier.
	//
	// +kubebuilder:validation:Enum=Unicast;VPNUnicast
	SAFI string `json:"safi"`
}

// +kubebuilder:object:root=true
type BGPProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPProvider{}, &BGPProviderList{})
}
