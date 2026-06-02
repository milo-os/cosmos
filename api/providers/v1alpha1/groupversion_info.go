// Package v1alpha1 contains API Schema definitions for the providers.bgp.miloapis.com/v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=providers.bgp.miloapis.com
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "providers.bgp.miloapis.com", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
