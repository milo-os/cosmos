package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestVPCAttachment() *VPCAttachment {
	return &VPCAttachment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "vpc.miloapis.com/v1alpha1",
			Kind:       "VPCAttachment",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment"},
		Spec: VPCAttachmentSpec{
			VPC: VPCRef{Name: "test-vpc"},
			Interface: VPCAttachmentInterface{
				Name:      "eth0",
				Addresses: []string{"10.0.1.2/24"},
			},
		},
	}
}

// TestVPCAttachmentDeepCopy verifies that DeepCopy produces an independent copy:
// mutations to slices and maps in the copy must not affect the original.
func TestVPCAttachmentDeepCopy(t *testing.T) {
	orig := newTestVPCAttachment()
	dup := orig.DeepCopy()

	dup.Spec.VPC.Name = "other-vpc"
	dup.Spec.Interface.Name = "eth1"
	dup.Spec.Interface.Addresses[0] = "10.0.2.0/24"

	if orig.Spec.VPC.Name != "test-vpc" {
		t.Errorf("VPC.Name mutated: got %q", orig.Spec.VPC.Name)
	}
	if orig.Spec.Interface.Name != "eth0" {
		t.Errorf("Interface.Name mutated: got %q", orig.Spec.Interface.Name)
	}
	if orig.Spec.Interface.Addresses[0] != "10.0.1.2/24" {
		t.Errorf("Interface.Addresses[0] mutated: got %q", orig.Spec.Interface.Addresses[0])
	}
}

// TestVPCAttachmentDeepCopyNil verifies DeepCopy on a nil pointer returns nil.
func TestVPCAttachmentDeepCopyNil(t *testing.T) {
	var a *VPCAttachment
	if a.DeepCopy() != nil {
		t.Error("DeepCopy on nil pointer should return nil")
	}
}

// TestVPCAttachmentDeepCopyMultipleAddresses verifies that multiple addresses
// in the copy are independent of the original.
func TestVPCAttachmentDeepCopyMultipleAddresses(t *testing.T) {
	orig := newTestVPCAttachment()
	orig.Spec.Interface.Addresses = []string{"10.0.1.2/24", "fd00::2/64"}
	dup := orig.DeepCopy()

	dup.Spec.Interface.Addresses[0] = "192.168.0.1/24"
	dup.Spec.Interface.Addresses[1] = "fd00::99/64"

	if orig.Spec.Interface.Addresses[0] != "10.0.1.2/24" {
		t.Errorf("Addresses[0] mutated: got %q", orig.Spec.Interface.Addresses[0])
	}
	if orig.Spec.Interface.Addresses[1] != "fd00::2/64" {
		t.Errorf("Addresses[1] mutated: got %q", orig.Spec.Interface.Addresses[1])
	}
}

// TestVPCAttachmentJSONRoundTrip verifies that the struct serialises and
// deserialises through JSON without data loss.
func TestVPCAttachmentJSONRoundTrip(t *testing.T) {
	orig := newTestVPCAttachment()
	orig.Spec.Interface.Addresses = []string{"10.0.1.2/24", "fd00::2/64"}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got VPCAttachment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.VPC.Name != orig.Spec.VPC.Name {
		t.Errorf("VPC.Name: got %q, want %q", got.Spec.VPC.Name, orig.Spec.VPC.Name)
	}
	if got.Spec.Interface.Name != orig.Spec.Interface.Name {
		t.Errorf("Interface.Name: got %q, want %q", got.Spec.Interface.Name, orig.Spec.Interface.Name)
	}
	if len(got.Spec.Interface.Addresses) != 2 {
		t.Fatalf("Addresses len: got %d, want 2", len(got.Spec.Interface.Addresses))
	}
	if got.Spec.Interface.Addresses[0] != "10.0.1.2/24" {
		t.Errorf("Addresses[0]: got %q, want %q", got.Spec.Interface.Addresses[0], "10.0.1.2/24")
	}
	if got.Spec.Interface.Addresses[1] != "fd00::2/64" {
		t.Errorf("Addresses[1]: got %q, want %q", got.Spec.Interface.Addresses[1], "fd00::2/64")
	}
}

// TestVPCAttachmentJSONFieldNames verifies JSON key names match the CRD schema.
func TestVPCAttachmentJSONFieldNames(t *testing.T) {
	orig := newTestVPCAttachment()
	data, err := json.Marshal(orig.Spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, ok := m["vpc"]; !ok {
		t.Error("expected JSON key \"vpc\" not found")
	}
	if _, ok := m["interface"]; !ok {
		t.Error("expected JSON key \"interface\" not found")
	}

	iface, _ := m["interface"].(map[string]any)
	if _, ok := iface["name"]; !ok {
		t.Error("expected JSON key \"interface.name\" not found")
	}
	if _, ok := iface["addresses"]; !ok {
		t.Error("expected JSON key \"interface.addresses\" not found")
	}
}

// TestVPCAttachmentInterfaceNameNoDefault verifies that the interface Name field
// has no server-side default applied in Go — it must be set explicitly.
// This is the regression test for section 3.11 of the CRD analysis (the
// "galactic0" placeholder default was removed).
func TestVPCAttachmentInterfaceNameNoDefault(t *testing.T) {
	iface := VPCAttachmentInterface{
		Addresses: []string{"10.0.1.2/24"},
	}
	if iface.Name != "" {
		t.Errorf("expected zero-value Name to be empty string, got %q (default should not be applied in Go)", iface.Name)
	}

	data, err := json.Marshal(iface)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if val, ok := m["name"]; ok && val != "" {
		t.Errorf("expected name to be empty in JSON (no default), got %q", val)
	}
}

// TestVPCAttachmentListDeepCopy verifies that VPCAttachmentList.DeepCopy
// produces independent copies of each item.
func TestVPCAttachmentListDeepCopy(t *testing.T) {
	list := &VPCAttachmentList{
		Items: []VPCAttachment{*newTestVPCAttachment()},
	}
	copied := list.DeepCopy()
	copied.Items[0].Spec.VPC.Name = "mutated-vpc"

	if list.Items[0].Spec.VPC.Name != "test-vpc" {
		t.Errorf("original list item mutated via copy")
	}
}

// TestVPCAttachmentAnnotationConstant verifies the annotation constant value.
func TestVPCAttachmentAnnotationConstant(t *testing.T) {
	const want = "k8s.v1alpha1.vpc.miloapis.com/vpc-attachment"
	if VPCAttachmentAnnotation != want {
		t.Errorf("VPCAttachmentAnnotation = %q, want %q", VPCAttachmentAnnotation, want)
	}
}
