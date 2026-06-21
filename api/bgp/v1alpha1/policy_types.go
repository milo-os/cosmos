package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPPolicyDirection is the direction in which a BGPPolicy is applied.
//
// +kubebuilder:validation:Enum=import;export
type BGPPolicyDirection string

const (
	// BGPPolicyDirectionImport applies the policy to routes received from peers.
	BGPPolicyDirectionImport BGPPolicyDirection = "import"

	// BGPPolicyDirectionExport applies the policy to routes advertised to peers.
	BGPPolicyDirectionExport BGPPolicyDirection = "export"
)

// BGPPolicyAction is the disposition applied when a policy term matches.
//
// +kubebuilder:validation:Enum=permit;deny
type BGPPolicyAction string

const (
	// BGPPolicyActionPermit allows the route and optionally applies set actions.
	BGPPolicyActionPermit BGPPolicyAction = "permit"

	// BGPPolicyActionDeny drops the route. Set actions must not be specified.
	BGPPolicyActionDeny BGPPolicyAction = "deny"
)

// BGPPolicy defines composable, ordered routing policy statements applied to a
// BGPRouter in a specific direction (import or export). It binds to one or more
// BGPRouter instances via routerRef or routerSelector.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=bgpp
// +kubebuilder:printcolumn:name="DIRECTION",type="string",JSONPath=".spec.direction"
// +kubebuilder:printcolumn:name="TERMS",type="integer",JSONPath=".spec.terms"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type BGPPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPPolicySpec   `json:"spec,omitempty"`
	Status BGPPolicyStatus `json:"status,omitempty"`
}

// BGPPolicySpec defines the desired route policy state.
//
// +kubebuilder:validation:XValidation:rule="self.terms.all(t1, self.terms.filter(t2, t2.sequence == t1.sequence).size() == 1)",message="Term sequence numbers must be unique"
type BGPPolicySpec struct {
	RouterTarget `json:",inline"`

	// Direction is the policy direction: import or export.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=import;export
	Direction BGPPolicyDirection `json:"direction"`

	// Terms is the ordered list of policy statements.
	// Evaluated from lowest to highest sequence number.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	Terms []BGPPolicyTerm `json:"terms"`
}

// BGPPolicyTerm is a single ordered policy statement with match conditions and an action.
//
// +kubebuilder:validation:XValidation:rule="self.action == 'deny' ? !has(self.set) : true",message="set actions are not permitted on deny terms"
type BGPPolicyTerm struct {
	// Sequence is the evaluation order. Lower values are evaluated first.
	// Must be unique within the policy.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Sequence int32 `json:"sequence"`

	// Match defines the conditions under which this term fires.
	Match BGPPolicyMatch `json:"match"`

	// Action is the disposition when this term matches.
	// +kubebuilder:validation:Enum=permit;deny
	Action BGPPolicyAction `json:"action"`

	// Set defines mutations applied when action is "permit".
	// Must not be set when action is "deny".
	// +optional
	Set *PolicySetActions `json:"set,omitempty"`
}

// BGPPolicyMatch defines the conditions under which a policy term fires.
//
// +kubebuilder:validation:XValidation:rule="!has(self.any) || !self.any || !has(self.addressFamilies) || self.addressFamilies.size() == 0",message="addressFamilies must be empty when any is true"
type BGPPolicyMatch struct {
	// Any matches all routes. When true, all other match fields are ignored.
	// +optional
	Any bool `json:"any,omitempty"`

	// AddressFamilies constrains the match to specific AFI/SAFI combinations.
	// If empty, all address families are matched.
	// +optional
	// +kubebuilder:validation:MaxItems=8
	AddressFamilies []AddressFamily `json:"addressFamilies,omitempty"`
}

// PolicySetActions defines mutations applied when a term matches with action "permit".
type PolicySetActions struct {
	// Communities defines community add/remove operations.
	// +optional
	Communities *CommunitySet `json:"communities,omitempty"`

	// LocalPreference sets the LOCAL_PREF attribute.
	// Only meaningful on import (iBGP) or export to iBGP peers.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	LocalPreference *uint32 `json:"localPreference,omitempty"`
}

// CommunitySet defines community add and remove operations.
type CommunitySet struct {
	// Add is a list of communities to attach.
	// +optional
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:items:MaxLength=24
	Add []string `json:"add,omitempty"`

	// Remove is a list of communities to strip.
	// +optional
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:items:MaxLength=24
	Remove []string `json:"remove,omitempty"`
}

// BGPPolicyStatus defines the observed state of BGPPolicy.
type BGPPolicyStatus struct {
	// ObservedGeneration is the .metadata.generation this status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions contains the standard conditions for this resource.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type BGPPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPPolicy{}, &BGPPolicyList{})
}
