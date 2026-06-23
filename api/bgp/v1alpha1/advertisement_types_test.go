package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newTestAdvertisement() *BGPAdvertisement {
	return &BGPAdvertisement{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPAdvertisement",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-adv"},
		Spec: BGPAdvertisementSpec{
			RouterRef:     RouterRef{Name: "test-router"},
			AddressFamily: AddressFamily{AFI: AFIIPv6, SAFI: SAFIUnicast},
			Prefixes: []AdvertisedPrefix{
				{CIDR: "2001:db8::/48"},
			},
		},
	}
}

// TestBGPAdvertisementDeepCopy verifies DeepCopy produces an independent copy.
func TestBGPAdvertisementDeepCopy(t *testing.T) {
	orig := newTestAdvertisement()
	orig.Spec.Communities = []string{"65000:100"}
	orig.Spec.LocalPreference = ptr(uint32(200))

	dup := orig.DeepCopy()

	dup.Spec.RouterRef.Name = "other-router"
	dup.Spec.Communities[0] = "65001:999"
	*dup.Spec.LocalPreference = 999

	if orig.Spec.RouterRef.Name != "test-router" {
		t.Errorf("RouterRef mutated: got %q", orig.Spec.RouterRef.Name)
	}
	if orig.Spec.Communities[0] != "65000:100" {
		t.Errorf("Communities[0] mutated: got %q", orig.Spec.Communities[0])
	}
	if *orig.Spec.LocalPreference != 200 {
		t.Errorf("LocalPreference mutated: got %d", *orig.Spec.LocalPreference)
	}
}

// TestBGPAdvertisementDeepCopyNil verifies DeepCopy on nil returns nil.
func TestBGPAdvertisementDeepCopyNil(t *testing.T) {
	var a *BGPAdvertisement
	if a.DeepCopy() != nil {
		t.Error("DeepCopy on nil should return nil")
	}
}

// TestBGPAdvertisementJSONRoundTrip verifies JSON serialisation round-trips correctly.
func TestBGPAdvertisementJSONRoundTrip(t *testing.T) {
	orig := newTestAdvertisement()
	orig.Spec.Prefixes = []AdvertisedPrefix{
		{CIDR: "2001:db8::/48", LocalPreference: ptr(uint32(150))},
		{CIDR: "10.0.0.0/8", Communities: []string{"65000:200"}},
	}
	orig.Spec.Communities = []string{"65000:100"}
	orig.Spec.LocalPreference = ptr(uint32(100))

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPAdvertisement
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.RouterRef.Name != orig.Spec.RouterRef.Name {
		t.Errorf("RouterRef: got %q, want %q", got.Spec.RouterRef.Name, orig.Spec.RouterRef.Name)
	}
	if len(got.Spec.Prefixes) != 2 {
		t.Fatalf("Prefixes len: got %d, want 2", len(got.Spec.Prefixes))
	}
	if got.Spec.Prefixes[0].CIDR != "2001:db8::/48" {
		t.Errorf("Prefixes[0].CIDR: got %q, want 2001:db8::/48", got.Spec.Prefixes[0].CIDR)
	}
	if got.Spec.Prefixes[0].LocalPreference == nil || *got.Spec.Prefixes[0].LocalPreference != 150 {
		t.Errorf("Prefixes[0].LocalPreference: got %v, want 150", got.Spec.Prefixes[0].LocalPreference)
	}
	if got.Spec.Prefixes[1].CIDR != "10.0.0.0/8" {
		t.Errorf("Prefixes[1].CIDR: got %q, want 10.0.0.0/8", got.Spec.Prefixes[1].CIDR)
	}
	if len(got.Spec.Prefixes[1].Communities) != 1 || got.Spec.Prefixes[1].Communities[0] != "65000:200" {
		t.Errorf("Prefixes[1].Communities: got %v, want [65000:200]", got.Spec.Prefixes[1].Communities)
	}
	if got.Spec.Communities[0] != "65000:100" {
		t.Errorf("Communities[0]: got %q, want 65000:100", got.Spec.Communities[0])
	}
	if got.Spec.LocalPreference == nil || *got.Spec.LocalPreference != 100 {
		t.Errorf("LocalPreference: got %v, want 100", got.Spec.LocalPreference)
	}
}

// TestBGPAdvertisementRedistributeJSONRoundTrip verifies redistribution sources
// serialise and deserialise correctly.
func TestBGPAdvertisementRedistributeJSONRoundTrip(t *testing.T) {
	orig := &BGPAdvertisement{
		TypeMeta:   metav1.TypeMeta{APIVersion: "bgp.miloapis.com/v1alpha1", Kind: "BGPAdvertisement"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-redistribute"},
		Spec: BGPAdvertisementSpec{
			RouterRef:     RouterRef{Name: "test-router"},
			AddressFamily: AddressFamily{AFI: AFIIPv4, SAFI: SAFIUnicast},
			Redistribute: []RedistributeSource{
				RedistributeSourceStatic,
				RedistributeSourceConnected,
			},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPAdvertisement
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(got.Spec.Redistribute) != 2 {
		t.Fatalf("Redistribute len: got %d, want 2", len(got.Spec.Redistribute))
	}
	if got.Spec.Redistribute[0] != RedistributeSourceStatic {
		t.Errorf("Redistribute[0]: got %q, want %q", got.Spec.Redistribute[0], RedistributeSourceStatic)
	}
	if got.Spec.Redistribute[1] != RedistributeSourceConnected {
		t.Errorf("Redistribute[1]: got %q, want %q", got.Spec.Redistribute[1], RedistributeSourceConnected)
	}
}

// TestBGPAdvertisementOriginateFromJSONRoundTrip verifies originateFrom serialises correctly.
func TestBGPAdvertisementOriginateFromJSONRoundTrip(t *testing.T) {
	orig := &BGPAdvertisement{
		TypeMeta:   metav1.TypeMeta{APIVersion: "bgp.miloapis.com/v1alpha1", Kind: "BGPAdvertisement"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-originate"},
		Spec: BGPAdvertisementSpec{
			RouterRef:     RouterRef{Name: "test-router"},
			AddressFamily: AddressFamily{AFI: AFIIPv6, SAFI: SAFIUnicast},
			OriginateFrom: &AdvertisementOriginateFrom{
				Type:          OriginateTypeInterface,
				InterfaceName: ptr("lo"),
			},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPAdvertisement
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.OriginateFrom == nil {
		t.Fatal("OriginateFrom is nil after round-trip")
	}
	if got.Spec.OriginateFrom.Type != OriginateTypeInterface {
		t.Errorf("OriginateFrom.Type: got %q, want %q", got.Spec.OriginateFrom.Type, OriginateTypeInterface)
	}
	if got.Spec.OriginateFrom.InterfaceName == nil || *got.Spec.OriginateFrom.InterfaceName != "lo" {
		t.Errorf("OriginateFrom.InterfaceName: got %v, want lo", got.Spec.OriginateFrom.InterfaceName)
	}
}

// TestBGPAdvertisementPolicyRefJSONRoundTrip verifies policyRef serialises correctly.
func TestBGPAdvertisementPolicyRefJSONRoundTrip(t *testing.T) {
	orig := newTestAdvertisement()
	orig.Spec.PolicyRef = &AdvertisementPolicyRef{Name: "export-filter"}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPAdvertisement
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.PolicyRef == nil {
		t.Fatal("PolicyRef is nil after round-trip")
	}
	if got.Spec.PolicyRef.Name != "export-filter" {
		t.Errorf("PolicyRef.Name: got %q, want export-filter", got.Spec.PolicyRef.Name)
	}
}

// TestBGPAdvertisementDeepCopyWithAllFields verifies DeepCopy correctly
// handles all pointer and slice fields.
func TestBGPAdvertisementDeepCopyWithAllFields(t *testing.T) {
	orig := &BGPAdvertisement{
		TypeMeta:   metav1.TypeMeta{APIVersion: "bgp.miloapis.com/v1alpha1", Kind: "BGPAdvertisement"},
		ObjectMeta: metav1.ObjectMeta{Name: "full-adv"},
		Spec: BGPAdvertisementSpec{
			RouterRef:     RouterRef{Name: "test-router"},
			AddressFamily: AddressFamily{AFI: AFIIPv6, SAFI: SAFIUnicast},
			Prefixes: []AdvertisedPrefix{
				{CIDR: "2001:db8::/48", Communities: []string{"65000:100"}, LocalPreference: ptr(uint32(100))},
			},
			Redistribute: []RedistributeSource{RedistributeSourceKernel},
			OriginateFrom: &AdvertisementOriginateFrom{
				Type:          OriginateTypeInterface,
				InterfaceName: ptr("eth0"),
			},
			PolicyRef:       &AdvertisementPolicyRef{Name: "my-policy"},
			Communities:     []string{"65000:99"},
			LocalPreference: ptr(uint32(50)),
		},
	}

	dup := orig.DeepCopy()

	// Mutate dup — original must be unaffected.
	dup.Spec.Prefixes[0].CIDR = "10.0.0.0/8"
	dup.Spec.Prefixes[0].Communities[0] = "99999:1"
	dup.Spec.Redistribute[0] = RedistributeSourceStatic
	dup.Spec.OriginateFrom.Type = OriginateTypeKernel
	*dup.Spec.OriginateFrom.InterfaceName = "eth1"
	dup.Spec.PolicyRef.Name = "other-policy"
	dup.Spec.Communities[0] = "99999:99"
	*dup.Spec.LocalPreference = 999

	if orig.Spec.Prefixes[0].CIDR != "2001:db8::/48" {
		t.Errorf("Prefixes[0].CIDR mutated: got %q", orig.Spec.Prefixes[0].CIDR)
	}
	if orig.Spec.Prefixes[0].Communities[0] != "65000:100" {
		t.Errorf("Prefixes[0].Communities[0] mutated: got %q", orig.Spec.Prefixes[0].Communities[0])
	}
	if orig.Spec.Redistribute[0] != RedistributeSourceKernel {
		t.Errorf("Redistribute[0] mutated: got %q", orig.Spec.Redistribute[0])
	}
	if orig.Spec.OriginateFrom.Type != OriginateTypeInterface {
		t.Errorf("OriginateFrom.Type mutated: got %q", orig.Spec.OriginateFrom.Type)
	}
	if *orig.Spec.OriginateFrom.InterfaceName != "eth0" {
		t.Errorf("OriginateFrom.InterfaceName mutated: got %q", *orig.Spec.OriginateFrom.InterfaceName)
	}
	if orig.Spec.PolicyRef.Name != "my-policy" {
		t.Errorf("PolicyRef.Name mutated: got %q", orig.Spec.PolicyRef.Name)
	}
	if orig.Spec.Communities[0] != "65000:99" {
		t.Errorf("Communities[0] mutated: got %q", orig.Spec.Communities[0])
	}
	if *orig.Spec.LocalPreference != 50 {
		t.Errorf("LocalPreference mutated: got %d", *orig.Spec.LocalPreference)
	}
}

// TestBGPAdvertisementListDeepCopy verifies BGPAdvertisementList.DeepCopy
// produces independent copies of each item.
func TestBGPAdvertisementListDeepCopy(t *testing.T) {
	list := &BGPAdvertisementList{
		Items: []BGPAdvertisement{*newTestAdvertisement()},
	}
	copied := list.DeepCopy()
	copied.Items[0].Spec.RouterRef.Name = "mutated"

	if list.Items[0].Spec.RouterRef.Name != "test-router" {
		t.Errorf("original list item mutated via copy")
	}
}

// TestAdvertisedPrefixJSONFieldNames verifies the JSON key names for AdvertisedPrefix.
func TestAdvertisedPrefixJSONFieldNames(t *testing.T) {
	p := AdvertisedPrefix{
		CIDR:            "2001:db8::/48",
		Communities:     []string{"65000:100"},
		LocalPreference: ptr(uint32(200)),
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{"cidr", "communities", "localPreference"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected JSON key %q not found in %v", key, m)
		}
	}
}

// TestRedistributeSourceConstants verifies enum values match expected strings.
func TestRedistributeSourceConstants(t *testing.T) {
	cases := []struct {
		src  RedistributeSource
		want string
	}{
		{RedistributeSourceStatic, "static"},
		{RedistributeSourceConnected, "connected"},
		{RedistributeSourceKernel, "kernel"},
	}
	for _, tt := range cases {
		if string(tt.src) != tt.want {
			t.Errorf("RedistributeSource %q: got %q, want %q", tt.src, string(tt.src), tt.want)
		}
	}
}

// TestOriginateTypeConstants verifies originate type enum values.
func TestOriginateTypeConstants(t *testing.T) {
	cases := []struct {
		typ  AdvertisementOriginateType
		want string
	}{
		{OriginateTypeInterface, "interface"},
		{OriginateTypeKernel, "kernel"},
	}
	for _, tt := range cases {
		if string(tt.typ) != tt.want {
			t.Errorf("AdvertisementOriginateType %q: got %q, want %q", tt.typ, string(tt.typ), tt.want)
		}
	}
}
