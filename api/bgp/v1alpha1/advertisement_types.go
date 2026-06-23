package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Community is a BGP community in ASN:NN or IP:NN format.
// +kubebuilder:validation:MaxLength=32
type Community string

// Prefix is an IPv4 or IPv6 CIDR prefix.
// +kubebuilder:validation:MaxLength=64
type Prefix string

// BGPAdvertisement defines routing information to advertise from a single BGPRouter.
// Prefixes are specified inline. Fan-out via routerSelector is intentionally not
// supported to avoid ambiguous prefix attribution across multiple routers.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=bgpadv
// +kubebuilder:printcolumn:name="ROUTER",type="string",JSONPath=".spec.routerRef.name"
// +kubebuilder:printcolumn:name="ADDRESS-FAMILY",type="string",JSONPath=".spec.addressFamily.afi"
// +kubebuilder:printcolumn:name="PREFIXES",type="integer",JSONPath=".status.advertisedPrefixes"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type BGPAdvertisement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPAdvertisementSpec   `json:"spec,omitempty"`
	Status BGPAdvertisementStatus `json:"status,omitempty"`
}

// BGPAdvertisementSpec defines the desired advertisement state.
type BGPAdvertisementSpec struct {
	// RouterRef targets a single BGPRouter by name.
	// +kubebuilder:validation:Required
	RouterRef RouterRef `json:"routerRef"`

	// AddressFamily defines the AFI/SAFI for this advertisement.
	// +kubebuilder:validation:Required
	AddressFamily AddressFamily `json:"addressFamily"`

	// Prefixes is the list of CIDR prefixes to advertise.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	// +kubebuilder:validation:items:MaxLength=43
	// +kubebuilder:validation:XValidation:rule="self.all(p, isCIDR(p))",message="each prefix must be a valid IPv4 or IPv6 CIDR"
	// +listType=set
	Prefixes []Prefix `json:"prefixes"`

	// Communities is the list of BGP communities to attach to advertised prefixes.
	// Format: ASN:NN or IP:NN.
	// +optional
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:items:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self.all(c, c.matches('^[0-9]{1,10}:[0-9]{1,10}$') || c.matches('^[0-9]{1,3}\\\\.[0-9]{1,3}\\\\.[0-9]{1,3}\\\\.[0-9]{1,3}:[0-9]{1,10}$'))",message="community must be in ASN:NN or IP:NN format"
	Communities []Community `json:"communities,omitempty"`

	// LocalPreference sets the BGP LOCAL_PREF attribute on advertised prefixes.
	// Only meaningful for iBGP sessions.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	LocalPreference *uint32 `json:"localPreference,omitempty"`
}

// BGPAdvertisementStatus defines the observed state of BGPAdvertisement.
type BGPAdvertisementStatus struct {
	// ObservedGeneration is the .metadata.generation this status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AdvertisedPrefixes is the count of prefixes currently being originated.
	// +optional
	AdvertisedPrefixes int32 `json:"advertisedPrefixes,omitempty"`

	// Conditions contains the standard conditions for this resource.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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
