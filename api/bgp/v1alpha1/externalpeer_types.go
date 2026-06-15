package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPExternalPeer is a registry entry for a BGP peer outside the cosmos-managed fleet.
// Referenced by BGPSession.spec.toPeers — the address and ASN are resolved at session write time.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgpep
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".spec.address"
// +kubebuilder:printcolumn:name="AS",type="integer",JSONPath=".spec.asNumber"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPExternalPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPExternalPeerSpec   `json:"spec,omitempty"`
	Status BGPExternalPeerStatus `json:"status,omitempty"`
}

// BGPExternalPeerSpec defines the external peer's address and ASN.
type BGPExternalPeerSpec struct {
	// Address is the peer's BGP address.
	//
	// +kubebuilder:validation:Format=ipv6
	Address string `json:"address"`

	// ASNumber is the peer's autonomous system number.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	ASNumber int64 `json:"asNumber"`

	// Description is a human-readable note about this peer.
	//
	// +kubebuilder:validation:MaxLength=256
	// +optional
	Description string `json:"description,omitempty"`
}

// BGPExternalPeerStatus defines the observed state of BGPExternalPeer.
type BGPExternalPeerStatus struct {
	// Conditions reflect the current state.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ReferencedBy is the count of BGPSession resources referencing this external peer.
	//
	// +optional
	ReferencedBy int32 `json:"referencedBy,omitempty"`

	// ReferencedByList names up to 50 referencing BGPSession resources.
	//
	// +optional
	ReferencedByList []string `json:"referencedByList,omitempty"`
}

// +kubebuilder:object:root=true
type BGPExternalPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPExternalPeer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPExternalPeer{}, &BGPExternalPeerList{})
}
