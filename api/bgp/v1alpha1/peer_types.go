package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPPeerState represents the BGP Finite State Machine state of a session.
type BGPPeerState string

const (
	BGPPeerStateIdle        BGPPeerState = "Idle"
	BGPPeerStateConnect     BGPPeerState = "Connect"
	BGPPeerStateActive      BGPPeerState = "Active"
	BGPPeerStateOpenSent    BGPPeerState = "OpenSent"
	BGPPeerStateOpenConfirm BGPPeerState = "OpenConfirm"
	BGPPeerStateEstablished BGPPeerState = "Established"
)

// BGPPeer defines a BGP session to a remote peer. It binds to one or more
// BGPRouter instances via routerRef or routerSelector.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=bgppr
// +kubebuilder:printcolumn:name="ROUTER",type="string",JSONPath=".spec.routerRef.name"
// +kubebuilder:printcolumn:name="PEER-ADDRESS",type="string",JSONPath=".spec.address"
// +kubebuilder:printcolumn:name="PEER-ASN",type="integer",JSONPath=".spec.peerASN"
// +kubebuilder:printcolumn:name="STATE",type="string",JSONPath=".status.sessionState"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type BGPPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPPeerSpec   `json:"spec,omitempty"`
	Status BGPPeerStatus `json:"status,omitempty"`
}

// BGPPeerSpec defines the desired state of BGPPeer.
type BGPPeerSpec struct {
	RouterTarget `json:",inline"`

	// PeerASN is the remote AS number.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	PeerASN uint32 `json:"peerASN"`

	// Address is the remote peer's IPv4 or IPv6 address.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="isIP(self)",message="address must be a valid IPv4 or IPv6 address"
	Address string `json:"address"`

	// Description is a human-readable label for this peer (e.g., "spine-1").
	// +optional
	Description string `json:"description,omitempty"`

	// AuthSecretRef references a Secret in the same namespace containing the
	// MD5 TCP authentication password under the key "password".
	// +optional
	AuthSecretRef *LocalSecretRef `json:"authSecretRef,omitempty"`

	// AddressFamilies defines the address families negotiated on this session.
	// +kubebuilder:validation:MinItems=1
	AddressFamilies []AddressFamily `json:"addressFamilies"`

	// HoldTime is the BGP hold timer. Must be 0 (disabled) or >= 3s.
	// Defaults to 90s if unset.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) == duration('0s') || duration(self) >= duration('3s')",message="holdTime must be 0 or >= 3s"
	HoldTime *metav1.Duration `json:"holdTime,omitempty"`

	// KeepaliveTime is the BGP keepalive interval. Must be <= HoldTime / 3.
	// Defaults to 30s if unset.
	// +optional
	KeepaliveTime *metav1.Duration `json:"keepaliveTime,omitempty"`
}

// BGPPeerStatus defines the observed state of BGPPeer.
type BGPPeerStatus struct {
	// ObservedGeneration is the .metadata.generation this status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SessionState is the current BGP FSM state of this session.
	// +optional
	SessionState BGPPeerState `json:"sessionState,omitempty"`

	// LastEstablishedTime is the timestamp of the most recent Established transition.
	// +optional
	LastEstablishedTime *metav1.Time `json:"lastEstablishedTime,omitempty"`

	// Conditions contains the standard conditions for this resource.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type BGPPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPPeer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPPeer{}, &BGPPeerList{})
}
