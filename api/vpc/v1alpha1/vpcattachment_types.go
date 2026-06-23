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

const VPCAttachmentAnnotation = "k8s.v1alpha1.vpc.miloapis.com/vpc-attachment"

// VPCAttachmentSpec defines the desired state of VPCAttachment
type VPCAttachmentSpec struct {
	// VPC this attachment belongs to.
	// +required
	VPC VPCRef `json:"vpc"`

	// Interface defines the network interface configuration.
	// +required
	Interface VPCAttachmentInterface `json:"interface"`
}

// VPCRef references a VPC by name within the same namespace.
type VPCRef struct {
	// Name is the name of the VPC.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// VPCAttachmentInterface defines the network interface details.
//
// +kubebuilder:validation:XValidation:rule="self.addresses.all(a, isCIDR(a))",message="each address must be a valid IPv4 or IPv6 CIDR"
type VPCAttachmentInterface struct {
	// Name of the interface (e.g., eth0).
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// A list of IPv4 or IPv6 addresses associated with the interface.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +required
	Addresses []string `json:"addresses"`
}

// VPCAttachmentStatus defines the observed state of VPCAttachment.
type VPCAttachmentStatus struct {
	// Indicates whether the VPCAttachment is ready for use
	// +required
	// +default:value=false
	Ready bool `json:"ready,omitempty"`

	// A unique identifier assigned to this VPCAttachment
	// +optional
	Identifier string `json:"identifier,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VPCAttachment is the Schema for the vpcattachments API
type VPCAttachment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of VPCAttachment
	// +required
	Spec VPCAttachmentSpec `json:"spec"`

	// status defines the observed state of VPCAttachment
	// +optional
	Status VPCAttachmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPCAttachmentList contains a list of VPCAttachments
type VPCAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCAttachment{}, &VPCAttachmentList{})
}
