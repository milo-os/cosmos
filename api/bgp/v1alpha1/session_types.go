package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPSession expresses bilateral BGP session intent.
// Written by the management cluster cosmos. Propagated by Karmada to member clusters.
// The receiving cluster's SessionReconciler generates BGPPeer resources locally.
// Self-contained: toPeers carries explicit resolved addresses, no cross-cluster lookups at reconcile time.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgps
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPSessionSpec   `json:"spec,omitempty"`
	Status BGPSessionStatus `json:"status,omitempty"`
}

// BGPSessionSpec defines the desired session state.
//
// +kubebuilder:validation:XValidation:rule="has(self.fromProviderSelector) != has(self.fromExternalPeerRef)",message="exactly one of fromProviderSelector or fromExternalPeerRef must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.fromProviderSelector) || (has(self.fromInstanceRef) && has(self.toPeers))",message="fromInstanceRef and toPeers are required when fromProviderSelector is set"
// +kubebuilder:validation:XValidation:rule="self.addressFamilies.size() > 0",message="at least one address family must be specified"
type BGPSessionSpec struct {
	// FromProviderSelector selects local BGPProvider resources representing the from-side of the session.
	// Mutually exclusive with fromExternalPeerRef.
	//
	// +optional
	FromProviderSelector *metav1.LabelSelector `json:"fromProviderSelector,omitempty"`

	// FromInstanceRef is the BGPInstance name to use for generated BGPPeer resources.
	// Required when fromProviderSelector is set.
	//
	// +optional
	FromInstanceRef string `json:"fromInstanceRef,omitempty"`

	// FromExternalPeerRef names a BGPExternalPeer resource representing the from-side.
	// No BGPPeer is generated on this side.
	// Mutually exclusive with fromProviderSelector.
	//
	// +optional
	FromExternalPeerRef *ExternalPeerRef `json:"fromExternalPeerRef,omitempty"`

	// ToPeers is the explicit list of peer addresses for the to-side of the session.
	// Resolved by management cluster cosmos at session write time.
	// The receiving cluster uses these directly without cross-cluster lookups.
	// Required when fromProviderSelector is set.
	//
	// +optional
	ToPeers []SessionPeer `json:"toPeers,omitempty"`

	// AddressFamilies are passed through to generated BGPPeer resources.
	//
	// +kubebuilder:validation:MinItems=1
	AddressFamilies []AddressFamily `json:"addressFamilies"`

	// Timers override the instance timer defaults for generated peers.
	//
	// +optional
	Timers *BGPPeerTimers `json:"timers,omitempty"`

	// AllowAsIn is passed through to generated BGPPeer resources.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	AllowAsIn *int32 `json:"allowAsIn,omitempty"`
}

// ExternalPeerRef names a BGPExternalPeer resource.
type ExternalPeerRef struct {
	// Name is the BGPExternalPeer resource name.
	Name string `json:"name"`
}

// SessionPeer is one entry in the toPeers list.
// Resolved from BGPProvider or BGPExternalPeer resources at session write time.
type SessionPeer struct {
	// Address is the explicit peer address.
	//
	// +kubebuilder:validation:Format=ipv6
	Address string `json:"address"`

	// ASNumber is the peer's autonomous system number.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	ASNumber int64 `json:"asNumber"`

	// InstanceRef is the BGPInstance name in this cluster to use for the generated BGPPeer.
	InstanceRef string `json:"instanceRef"`

	// RouteReflectorClient designates this peer as an RR client on the generated BGPPeer.
	// Set by management cluster cosmos when the to-side is an RR client designation.
	//
	// +optional
	RouteReflectorClient bool `json:"routeReflectorClient,omitempty"`

	// RemotePort is the TCP port to connect to on this peer.
	// Passed through to the generated BGPPeer. Defaults to 179 when unset.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	RemotePort *int32 `json:"remotePort,omitempty"`
}

// BGPSessionStatus defines the observed state of BGPSession.
type BGPSessionStatus struct {
	// Conditions reflect the current reconciliation state.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// FromSide summarizes the from-side reconciliation.
	//
	// +optional
	FromSide *SessionFromSideStatus `json:"fromSide,omitempty"`

	// ToSide summarizes the to-side peer list.
	//
	// +optional
	ToSide *SessionToSideStatus `json:"toSide,omitempty"`
}

// SessionFromSideStatus summarizes from-side provider matching.
type SessionFromSideStatus struct {
	// MatchedProviders is the count of local BGPProvider resources matched by fromProviderSelector.
	MatchedProviders int32 `json:"matchedProviders"`

	// GeneratedPeers is the count of BGPPeer resources generated.
	GeneratedPeers int32 `json:"generatedPeers"`
}

// SessionToSideStatus summarizes the to-side peer list.
type SessionToSideStatus struct {
	// PeerCount is the number of entries in spec.toPeers.
	PeerCount int32 `json:"peerCount"`
}

// +kubebuilder:object:root=true
type BGPSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPSession{}, &BGPSessionList{})
}
