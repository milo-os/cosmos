package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPAdvertisement injects infrastructure prefixes into the BGP RIB.
// Used for node loopback /128 and SRv6 locator prefix advertisement.
// Not used for per-workload or per-VRF routes — CNI owns those.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgpadv
// +kubebuilder:printcolumn:name="Instance",type="string",JSONPath=".spec.instanceRef"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPAdvertisement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPAdvertisementSpec   `json:"spec,omitempty"`
	Status BGPAdvertisementStatus `json:"status,omitempty"`
}

// BGPAdvertisementSpec defines the desired advertisement state.
type BGPAdvertisementSpec struct {
	// InstanceRef is the name of the BGPInstance to advertise through.
	// Must reference a BGPInstance bound to providers with Unicast address families.
	InstanceRef string `json:"instanceRef"`

	// Prefixes is the list of IPv4 or IPv6 unicast CIDR blocks to advertise.
	//
	// +kubebuilder:validation:MinItems=1
	Prefixes []string `json:"prefixes"`

	// PeerSelector restricts advertisement to matched BGPPeer resources.
	// If absent, advertise to all peers on matched providers.
	//
	// +optional
	PeerSelector *metav1.LabelSelector `json:"peerSelector,omitempty"`
}

// BGPAdvertisementStatus defines the observed state of BGPAdvertisement.
type BGPAdvertisementStatus struct {
	// Conditions are top-level conditions for this BGPAdvertisement.
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
type BGPAdvertisementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPAdvertisement `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPAdvertisement{}, &BGPAdvertisementList{})
}
