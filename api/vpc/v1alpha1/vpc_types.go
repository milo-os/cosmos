/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Network is an IPv4 or IPv6 CIDR network string.
//
// +kubebuilder:validation:MaxLength=64
type Network string

// VPCSpec defines the desired state of a VPC.
//
// +kubebuilder:validation:XValidation:rule="self.networks.all(n, isCIDR(n))",message="each network must be a valid IPv4 or IPv6 CIDR"
type VPCSpec struct {
	// A list of networks in IPv4 or IPv6 CIDR notation associated with the VPC
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	Networks []Network `json:"networks"`
}

// VPCStatus defines the observed state of a VPC
type VPCStatus struct {
	// Indicates whether the VPC is ready for use
	// +required
	// +default:value=false
	Ready bool `json:"ready,omitempty"`

	// A unique identifier assigned to this VPC
	// +optional
	Identifier string `json:"identifier,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// VPC is the Schema for the vpcs API
type VPC struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of a VPC
	// +required
	Spec VPCSpec `json:"spec"`

	// status defines the observed state of a VPC
	// +optional
	Status VPCStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPCList contains a list of VPCs
type VPCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPC{}, &VPCList{})
}
