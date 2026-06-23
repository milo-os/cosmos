package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BGPVRFInstance configures an L2VPN EVPN VRF on matched BGPRouters.
// The referenced BGPRouter must have l2vpn-evpn in its addressFamilies.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=bgpvrf
// +kubebuilder:printcolumn:name="RD",type="string",JSONPath=".spec.routeDistinguisher"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BGPVRFInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPVRFInstanceSpec   `json:"spec,omitempty"`
	Status BGPVRFInstanceStatus `json:"status,omitempty"`
}

// BGPVRFInstanceSpec defines the desired VRF configuration.
//
// +kubebuilder:validation:XValidation:rule="self.routeDistinguisher.matches('^([0-9]{1,9}[.][0-9]{1,9}[.][0-9]{1,9}[.][0-9]{1,9}|[0-9]{1,9}):[0-9]{1,9}$')",message="routeDistinguisher must be in ASN:NN or IP:NN format"
type BGPVRFInstanceSpec struct {
	RouterTarget `json:",inline"`

	// RouteDistinguisher uniquely identifies this VRF in the BGP control plane.
	// Format: "ASN:NN" (e.g. "65000:100") or "IP:NN" (e.g. "192.0.2.1:100").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=21
	RouteDistinguisher string `json:"routeDistinguisher"`

	// ImportRouteTargets is the list of BGP extended community route targets
	// used to import routes into this VRF.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ImportRouteTargets []RouteTarget `json:"importRouteTargets"`

	// ExportRouteTargets is the list of BGP extended community route targets
	// attached to routes exported from this VRF.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	ExportRouteTargets []RouteTarget `json:"exportRouteTargets"`
}

// RouteTarget is a BGP extended community in "ASN:NN" or "IP:NN" format.
//
// +kubebuilder:validation:XValidation:rule="self.value.matches('^([0-9]{1,9}[.][0-9]{1,9}[.][0-9]{1,9}[.][0-9]{1,9}|[0-9]{1,9}):[0-9]{1,9}$')",message="value must be in ASN:NN or IP:NN format"
type RouteTarget struct {
	// Value is the route target extended community string.
	// Format: "ASN:NN" (e.g. "65000:100") or "IP:NN" (e.g. "192.0.2.1:100").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=21
	Value string `json:"value"`
}

// EVPNRouteType identifies one of the five EVPN BGP route types (RFC 7432, RFC 8365).
//
// +kubebuilder:validation:Enum=InclusiveMulticastEthernetTag;MACIPAdvertisement;IPPrefixAdvertisement;StickyMACAddress;IPv6PrefixAdvertisement
type EVPNRouteType string

const (
	// EVPNRouteTypeInclusiveMulticastEthernetTag is EVPN Type 1 — used for BUM traffic distribution.
	EVPNRouteTypeInclusiveMulticastEthernetTag EVPNRouteType = "InclusiveMulticastEthernetTag"

	// EVPNRouteTypeMACIPAdvertisement is EVPN Type 2 — host MAC/IP reachability.
	EVPNRouteTypeMACIPAdvertisement EVPNRouteType = "MACIPAdvertisement"

	// EVPNRouteTypeIPPrefixAdvertisement is EVPN Type 3 — L3 route distribution (IPv4).
	EVPNRouteTypeIPPrefixAdvertisement EVPNRouteType = "IPPrefixAdvertisement"

	// EVPNRouteTypeStickyMACAddress is EVPN Type 4 — static/sticky MAC mobility.
	EVPNRouteTypeStickyMACAddress EVPNRouteType = "StickyMACAddress"

	// EVPNRouteTypeIPv6PrefixAdvertisement is EVPN Type 5 — L3 route distribution (IPv6).
	EVPNRouteTypeIPv6PrefixAdvertisement EVPNRouteType = "IPv6PrefixAdvertisement"
)

// EVPNRouteCounter is a per-route-type EVPN route count reported in BGPVRFInstance status.
type EVPNRouteCounter struct {
	// RouteType is the EVPN BGP route type.
	RouteType EVPNRouteType `json:"routeType"`

	// Count is the number of active routes of this type.
	// +kubebuilder:validation:Minimum=0
	Count int64 `json:"count"`
}

// BGPVRFInstanceStatus defines the observed state of BGPVRFInstance.
type BGPVRFInstanceStatus struct {
	// Conditions are top-level conditions for this BGPVRFInstance.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Routers holds per-router reconciliation status.
	//
	// +listType=map
	// +listMapKey=routerName
	// +optional
	Routers []RouterStatus `json:"routers,omitempty"`

	// VNI is the VXLAN Network Identifier applied to this VRF, as reported by the BGP runtime.
	// Populated once the controller has successfully configured the VRF on at least one router.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16777215
	VNI *uint32 `json:"vni,omitempty"`

	// EVPNRouteCount is the per-route-type count of active EVPN routes in this VRF.
	//
	// +listType=map
	// +listMapKey=routeType
	// +optional
	EVPNRouteCount []EVPNRouteCounter `json:"evpnRouteCount,omitempty"`
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
