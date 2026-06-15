package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPPeer configures one side of a BGP session on matched providers.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgppr
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".spec.address"
// +kubebuilder:printcolumn:name="AS",type="integer",JSONPath=".spec.asNumber"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPPeerSpec   `json:"spec,omitempty"`
	Status BGPPeerStatus `json:"status,omitempty"`
}

// BGPPeerSpec defines the desired state of BGPPeer.
//
// +kubebuilder:validation:XValidation:rule="has(self.providerRef) != has(self.providerSelector)",message="exactly one of providerRef or providerSelector must be set"
// +kubebuilder:validation:XValidation:rule="!(has(self.ebgpMultihop) && has(self.ttlSecurity))",message="ebgpMultihop and ttlSecurity are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.passwordSecretRef) || (has(self.passwordSecretRef.name) && has(self.passwordSecretRef.key))",message="passwordSecretRef requires both name and key"
type BGPPeerSpec struct {
	// InstanceRef is the name of the BGPInstance for this peer session.
	InstanceRef string `json:"instanceRef"`

	// ProviderRef names a single BGPProvider.
	// Use for topology-specific sessions (underlay eBGP, RR client designations).
	// Mutually exclusive with providerSelector.
	//
	// +optional
	ProviderRef string `json:"providerRef,omitempty"`

	// ProviderSelector selects multiple BGPProvider resources.
	// Use for cluster-wide sessions (e.g. all overlay providers peer with same RRs).
	// Mutually exclusive with providerRef.
	//
	// +optional
	ProviderSelector *metav1.LabelSelector `json:"providerSelector,omitempty"`

	// Address is the peer's BGP address.
	//
	// +kubebuilder:validation:Format=ipv6
	Address string `json:"address"`

	// ASNumber is the peer's autonomous system number.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	ASNumber int64 `json:"asNumber"`

	// AddressFamilies overrides the instance address families for this peer.
	// If absent, inherited from the referenced BGPInstance.
	//
	// +optional
	AddressFamilies []AddressFamily `json:"addressFamilies,omitempty"`

	// Timers overrides the instance timer defaults for this peer.
	//
	// +optional
	Timers *BGPPeerTimers `json:"timers,omitempty"`

	// AllowAsIn permits the peer's AS number to appear in received AS paths.
	// Used on underlay eBGP sessions with a global AS.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	AllowAsIn *int32 `json:"allowAsIn,omitempty"`

	// RouteReflectorClient designates the remote peer as an RR client.
	// Set on RR-side peers only. Never on client-side peers.
	//
	// +optional
	RouteReflectorClient bool `json:"routeReflectorClient,omitempty"`

	// Passive enables passive mode — do not initiate the TCP connection.
	//
	// +optional
	Passive bool `json:"passive,omitempty"`

	// EBGPMultihop sets the eBGP multihop TTL.
	// Mutually exclusive with ttlSecurity. Invalid on iBGP sessions.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=255
	// +optional
	EBGPMultihop *int32 `json:"ebgpMultihop,omitempty"`

	// TTLSecurity sets the TTL security hop count.
	// Mutually exclusive with ebgpMultihop.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=254
	// +optional
	TTLSecurity *int32 `json:"ttlSecurity,omitempty"`

	// RemotePort is the TCP port to connect to on the remote peer.
	// Defaults to 179 (standard BGP) when unset.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	RemotePort *int32 `json:"remotePort,omitempty"`

	// PasswordSecretRef references a Secret containing the BGP session password.
	//
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// BGPPeerTimers holds per-peer BGP timer overrides.
type BGPPeerTimers struct {
	// HoldTime overrides the instance default hold time.
	//
	// +kubebuilder:validation:Minimum=3
	// +optional
	HoldTime *int32 `json:"holdTime,omitempty"`

	// Keepalive overrides the instance default keepalive interval.
	//
	// +kubebuilder:validation:Minimum=1
	// +optional
	Keepalive *int32 `json:"keepalive,omitempty"`
}

// SecretKeyRef references a key in a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Secret.
	Name string `json:"name"`

	// Key is the key within the Secret.
	Key string `json:"key"`
}

// BGPPeerStatus defines the observed state of BGPPeer.
type BGPPeerStatus struct {
	// Conditions are top-level conditions for this BGPPeer.
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
type BGPPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPPeer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPPeer{}, &BGPPeerList{})
}
