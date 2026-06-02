package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPRoutePolicy applies import/export route policy to selected BGPPeer resources.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgprp
// +kubebuilder:printcolumn:name="Instance",type="string",JSONPath=".spec.instanceRef"
// +kubebuilder:printcolumn:name="Priority",type="integer",JSONPath=".spec.priority"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPRoutePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPRoutePolicySpec   `json:"spec,omitempty"`
	Status BGPRoutePolicyStatus `json:"status,omitempty"`
}

// BGPRoutePolicySpec defines the desired route policy state.
type BGPRoutePolicySpec struct {
	// InstanceRef is the name of the BGPInstance this policy applies to.
	InstanceRef string `json:"instanceRef"`

	// PeerSelector selects BGPPeer resources this policy applies to.
	//
	// +optional
	PeerSelector *metav1.LabelSelector `json:"peerSelector,omitempty"`

	// Priority determines policy ordering. Higher priority policies are applied first.
	// Equal priority: sorted by metadata.name ascending.
	//
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// ImportStatements are evaluated on routes received from peers.
	//
	// +optional
	ImportStatements []PolicyStatement `json:"importStatements,omitempty"`

	// ExportStatements are evaluated on routes sent to peers.
	//
	// +optional
	ExportStatements []PolicyStatement `json:"exportStatements,omitempty"`
}

// PolicyStatement is a single route policy statement with match conditions and actions.
type PolicyStatement struct {
	// Name is the statement identifier.
	Name string `json:"name"`

	// Conditions define what routes this statement matches.
	//
	// +optional
	Conditions *PolicyConditions `json:"conditions,omitempty"`

	// Actions define what to do with matched routes.
	Actions PolicyActions `json:"actions"`
}

// PolicyConditions contains route match conditions.
type PolicyConditions struct {
	// PrefixSets is a list of prefix set names to match.
	// +optional
	PrefixSets []string `json:"prefixSets,omitempty"`

	// CommunitySet is a community set name to match.
	// +optional
	CommunitySet string `json:"communitySet,omitempty"`

	// NextHopSet is a next-hop set name to match.
	// +optional
	NextHopSet string `json:"nextHopSet,omitempty"`
}

// PolicyActions defines what to do with a matched route.
type PolicyActions struct {
	// RouteDisposition is the action to take: accept or reject.
	//
	// +kubebuilder:validation:Enum=Accept;Reject
	RouteDisposition string `json:"routeDisposition"`

	// SetCommunity adds or replaces BGP communities.
	//
	// +optional
	SetCommunity *SetCommunityAction `json:"setCommunity,omitempty"`

	// SetLocalPreference sets the local preference attribute.
	//
	// +optional
	SetLocalPreference *int32 `json:"setLocalPreference,omitempty"`

	// SetMED sets the Multi-Exit Discriminator attribute.
	//
	// +optional
	SetMED *int32 `json:"setMED,omitempty"`

	// SetNextHop sets the next-hop address.
	//
	// +optional
	SetNextHop string `json:"setNextHop,omitempty"`
}

// SetCommunityAction specifies how to modify BGP communities.
type SetCommunityAction struct {
	// Communities is the list of community values to set.
	Communities []string `json:"communities"`

	// Method controls whether communities are added to or replace existing ones.
	//
	// +kubebuilder:validation:Enum=Add;Replace;Remove
	Method string `json:"method"`
}

// BGPRoutePolicyStatus defines the observed state of BGPRoutePolicy.
type BGPRoutePolicyStatus struct {
	// Providers holds per-provider reconciliation status.
	//
	// +listType=map
	// +listMapKey=providerName
	// +optional
	Providers []ProviderStatus `json:"providers,omitempty"`
}

// +kubebuilder:object:root=true
type BGPRoutePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPRoutePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPRoutePolicy{}, &BGPRoutePolicyList{})
}
