package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPVRFInstance configures an L2VPN EVPN VRF on providers matched by
// spec.providerSelector. The referenced BGPInstance must have L2VPN/EVPN in
// its addressFamilies.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bgpvrf
// +kubebuilder:printcolumn:name="RD",type="string",JSONPath=".spec.routeDistinguisher"
// +kubebuilder:printcolumn:name="Instance",type="string",JSONPath=".spec.instanceRef"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPVRFInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPVRFInstanceSpec   `json:"spec,omitempty"`
	Status BGPVRFInstanceStatus `json:"status,omitempty"`
}

// BGPVRFInstanceSpec defines the desired VRF configuration.
//
// +kubebuilder:validation:XValidation:rule="self.routeDistinguisher.matches('^([0-9]{1,3}[.][0-9]{1,3}[.][0-9]{1,3}[.][0-9]{1,3}|[0-9]{1,10}):[0-9]{1,10}$')",message="routeDistinguisher must be in ASN:NN or IP:NN format"
type BGPVRFInstanceSpec struct {
	// InstanceRef is the name of the BGPInstance this VRF is associated with.
	// The referenced instance must have L2VPN/EVPN in its addressFamilies.
	InstanceRef string `json:"instanceRef"`

	// ProviderSelector selects the BGPProvider resources to configure this VRF on.
	// The matched providers must support the L2VPN/EVPN address family.
	ProviderSelector metav1.LabelSelector `json:"providerSelector"`

	// RouteDistinguisher uniquely identifies this VRF in the BGP control plane.
	// Format: "ASN:NN" (e.g. "65000:100") or "IP:NN" (e.g. "192.0.2.1:100").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=21
	RouteDistinguisher string `json:"routeDistinguisher"`

	// ImportRouteTargets is the list of BGP extended community route targets
	// used to import routes into this VRF.
	// Format per entry: "ASN:NN" or "IP:NN".
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ImportRouteTargets []RouteTarget `json:"importRouteTargets"`

	// ExportRouteTargets is the list of BGP extended community route targets
	// attached to routes exported from this VRF.
	// Format per entry: "ASN:NN" or "IP:NN".
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ExportRouteTargets []RouteTarget `json:"exportRouteTargets"`
}

// RouteTarget is a BGP extended community in "ASN:NN" or "IP:NN" format.
//
// +kubebuilder:validation:XValidation:rule="self.value.matches('^([0-9]{1,3}[.][0-9]{1,3}[.][0-9]{1,3}[.][0-9]{1,3}|[0-9]{1,10}):[0-9]{1,10}$')",message="value must be in ASN:NN or IP:NN format"
type RouteTarget struct {
	// Value is the route target extended community string.
	// Format: "ASN:NN" (e.g. "65000:100") or "IP:NN" (e.g. "192.0.2.1:100").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=21
	Value string `json:"value"`
}

// BGPVRFInstanceStatus defines the observed state of BGPVRFInstance.
type BGPVRFInstanceStatus struct {
	// Conditions are top-level conditions for this BGPVRFInstance.
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
type BGPVRFInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPVRFInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BGPVRFInstance{}, &BGPVRFInstanceList{})
}
