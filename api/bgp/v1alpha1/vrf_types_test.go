package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestVRF() *BGPVRFInstance {
	return &BGPVRFInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPVRFInstance",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-vrf"},
		Spec: BGPVRFInstanceSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "overlay-router"},
			},
			RouteDistinguisher: "65000:100",
			ImportRouteTargets: []RouteTarget{{Value: "65000:100"}},
			ExportRouteTargets: []RouteTarget{{Value: "65000:100"}},
		},
	}
}

// TestBGPVRFInstanceDeepCopy verifies that DeepCopy produces an independent copy:
// mutations to slices and maps in the copy must not affect the original.
func TestBGPVRFInstanceDeepCopy(t *testing.T) {
	orig := newTestVRF()
	dup := orig.DeepCopy()

	// Mutate dup — original must be unaffected.
	dup.Spec.RouteDistinguisher = "65001:200"
	dup.Spec.ImportRouteTargets[0].Value = "65001:200"
	dup.Spec.RouterRef.Name = "other-router"

	if orig.Spec.RouteDistinguisher != "65000:100" {
		t.Errorf("RouteDistinguisher mutated: got %q", orig.Spec.RouteDistinguisher)
	}
	if orig.Spec.ImportRouteTargets[0].Value != "65000:100" {
		t.Errorf("ImportRouteTargets[0] mutated: got %q", orig.Spec.ImportRouteTargets[0].Value)
	}
	if orig.Spec.RouterRef.Name != "overlay-router" {
		t.Errorf("RouterRef mutated: got %q", orig.Spec.RouterRef.Name)
	}
}

// TestBGPVRFInstanceDeepCopyNil verifies DeepCopy on a nil pointer returns nil.
func TestBGPVRFInstanceDeepCopyNil(t *testing.T) {
	var v *BGPVRFInstance
	if v.DeepCopy() != nil {
		t.Error("DeepCopy on nil pointer should return nil")
	}
}

// TestBGPVRFInstanceJSONRoundTrip verifies that the struct serialises and
// deserialises through JSON without data loss.
func TestBGPVRFInstanceJSONRoundTrip(t *testing.T) {
	orig := newTestVRF()
	orig.Spec.ImportRouteTargets = []RouteTarget{{Value: "65000:100"}, {Value: "65000:200"}}
	orig.Spec.ExportRouteTargets = []RouteTarget{{Value: "65000:100"}}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPVRFInstance
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.RouteDistinguisher != orig.Spec.RouteDistinguisher {
		t.Errorf("RD: got %q, want %q", got.Spec.RouteDistinguisher, orig.Spec.RouteDistinguisher)
	}
	if got.Spec.RouterRef.Name != orig.Spec.RouterRef.Name {
		t.Errorf("RouterRef: got %q, want %q", got.Spec.RouterRef.Name, orig.Spec.RouterRef.Name)
	}
	if len(got.Spec.ImportRouteTargets) != 2 {
		t.Errorf("ImportRouteTargets len: got %d, want 2", len(got.Spec.ImportRouteTargets))
	}
	if len(got.Spec.ExportRouteTargets) != 1 {
		t.Errorf("ExportRouteTargets len: got %d, want 1", len(got.Spec.ExportRouteTargets))
	}
	if got.Spec.ExportRouteTargets[0].Value != "65000:100" {
		t.Errorf("ExportRouteTargets[0]: got %q, want %q", got.Spec.ExportRouteTargets[0].Value, "65000:100")
	}
}

// TestBGPVRFInstanceListDeepCopy verifies that BGPVRFInstanceList.DeepCopy
// produces independent copies of each item.
func TestBGPVRFInstanceListDeepCopy(t *testing.T) {
	list := &BGPVRFInstanceList{
		Items: []BGPVRFInstance{*newTestVRF()},
	}
	copied := list.DeepCopy()
	copied.Items[0].Spec.RouteDistinguisher = "99:99"

	if list.Items[0].Spec.RouteDistinguisher != "65000:100" {
		t.Errorf("original list item mutated via copy")
	}
}

// TestRouteTargetJSONFieldName verifies the JSON field name for RouteTarget.Value
// matches the CRD schema ("value").
func TestRouteTargetJSONFieldName(t *testing.T) {
	rt := RouteTarget{Value: "65000:100"}
	data, err := json.Marshal(rt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v, ok := m["value"]; !ok || v != "65000:100" {
		t.Errorf("expected JSON key \"value\"=\"65000:100\", got %v", m)
	}
}

// TestBGPVRFInstanceStatusEVPNRouteCountsJSONRoundTrip verifies the new
// EVPN route counts field survives JSON serialization.
func TestBGPVRFInstanceStatusEVPNRouteCountsJSONRoundTrip(t *testing.T) {
	status := BGPVRFInstanceStatus{
		EVPNRouteCounts: []EVPNRouteCount{
			{RouteType: EVPNRouteTypeMACIPAdvertisement, Count: 150},
			{RouteType: EVPNRouteTypeIPPrefixAdvertisement, Count: 80},
			{RouteType: EVPNRouteTypeInclusiveMulticastEthernetTag, Count: 1},
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPVRFInstanceStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(got.EVPNRouteCounts) != 3 {
		t.Fatalf("EVPNRouteCounts count: got %d, want 3", len(got.EVPNRouteCounts))
	}
	if got.EVPNRouteCounts[0].RouteType != EVPNRouteTypeMACIPAdvertisement {
		t.Errorf("EVPNRouteCounts[0].RouteType: got %q, want %q",
			got.EVPNRouteCounts[0].RouteType, EVPNRouteTypeMACIPAdvertisement)
	}
	if got.EVPNRouteCounts[0].Count != 150 {
		t.Errorf("EVPNRouteCounts[0].Count: got %d, want 150", got.EVPNRouteCounts[0].Count)
	}
	if got.EVPNRouteCounts[2].RouteType != EVPNRouteTypeInclusiveMulticastEthernetTag {
		t.Errorf("EVPNRouteCounts[2].RouteType: got %q, want %q",
			got.EVPNRouteCounts[2].RouteType, EVPNRouteTypeInclusiveMulticastEthernetTag)
	}
}

// TestBGPVRFInstanceStatusEVPNRouteCountsJSONKeys verifies the JSON key names
// for the new status fields.
func TestBGPVRFInstanceStatusEVPNRouteCountsJSONKeys(t *testing.T) {
	status := BGPVRFInstanceStatus{
		EVPNRouteCounts: []EVPNRouteCount{
			{RouteType: EVPNRouteTypeMACIPAdvertisement, Count: 42},
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	raw, ok := m["evpnRouteCounts"]
	if !ok {
		t.Fatal("expected JSON key \"evpnRouteCounts\" not found")
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected evpnRouteCounts to be an array, got %T", raw)
	}
	if len(arr) != 1 {
		t.Errorf("expected evpnRouteCounts array length 1, got %d", len(arr))
	}
}

// TestBGPVRFInstanceStatusDeepCopyWithEVPNRouteCounts verifies DeepCopy handles
// the new EVPN route counts field correctly.
func TestBGPVRFInstanceStatusDeepCopyWithEVPNRouteCounts(t *testing.T) {
	orig := &BGPVRFInstanceStatus{
		EVPNRouteCounts: []EVPNRouteCount{
			{RouteType: EVPNRouteTypeMACIPAdvertisement, Count: 100},
			{RouteType: EVPNRouteTypeIPPrefixAdvertisement, Count: 50},
		},
	}

	copied := orig.DeepCopy()

	copied.EVPNRouteCounts[0].Count = 999
	copied.EVPNRouteCounts = append(copied.EVPNRouteCounts,
		EVPNRouteCount{RouteType: EVPNRouteTypeStickyMACAddress, Count: 5})

	if orig.EVPNRouteCounts[0].Count != 100 {
		t.Errorf("EVPNRouteCounts[0].Count mutated via copy: got %d, want 100",
			orig.EVPNRouteCounts[0].Count)
	}
	if len(orig.EVPNRouteCounts) != 2 {
		t.Errorf("EVPNRouteCounts length mutated via copy: got %d, want 2",
			len(orig.EVPNRouteCounts))
	}
}
