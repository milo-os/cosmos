package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestPeer() *BGPPeer {
	return &BGPPeer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPPeer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-peer"},
		Spec: BGPPeerSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "test-router"},
			},
			PeerASN: 65000,
			Address: "10.0.0.2",
			AddressFamilies: []AddressFamily{
				{
					AFI:  AFIIPv4,
					SAFI: SAFIUnicast,
				},
			},
		},
	}
}

// TestBGPPeerDeepCopy verifies that DeepCopy produces an independent copy.
func TestBGPPeerDeepCopy(t *testing.T) {
	orig := newTestPeer()
	dup := orig.DeepCopy()

	dup.Spec.PeerASN = 65001
	dup.Spec.Address = "10.0.0.3"
	dup.Spec.Description = "mutated"

	if orig.Spec.PeerASN != 65000 {
		t.Errorf("PeerASN mutated: got %d, want 65000", orig.Spec.PeerASN)
	}
	if orig.Spec.Address != "10.0.0.2" {
		t.Errorf("Address mutated: got %q", orig.Spec.Address)
	}
	if orig.Spec.Description != "" {
		t.Errorf("Description mutated: got %q", orig.Spec.Description)
	}
}

// TestBGPPeerDeepCopyNil verifies DeepCopy on a nil pointer returns nil.
func TestBGPPeerDeepCopyNil(t *testing.T) {
	var p *BGPPeer
	if p.DeepCopy() != nil {
		t.Error("DeepCopy on nil pointer should return nil")
	}
}

// TestBGPPeerJSONRoundTrip verifies that the struct serialises and
// deserialises through JSON without data loss.
func TestBGPPeerJSONRoundTrip(t *testing.T) {
	orig := newTestPeer()
	orig.Spec.Description = "spine-1"

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPPeer
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.PeerASN != orig.Spec.PeerASN {
		t.Errorf("PeerASN: got %d, want %d", got.Spec.PeerASN, orig.Spec.PeerASN)
	}
	if got.Spec.Address != orig.Spec.Address {
		t.Errorf("Address: got %q, want %q", got.Spec.Address, orig.Spec.Address)
	}
	if got.Spec.Description != orig.Spec.Description {
		t.Errorf("Description: got %q, want %q", got.Spec.Description, orig.Spec.Description)
	}
}

// TestBGPPeerListDeepCopy verifies that BGPPeerList.DeepCopy produces
// independent copies of each item.
func TestBGPPeerListDeepCopy(t *testing.T) {
	list := &BGPPeerList{
		Items: []BGPPeer{*newTestPeer()},
	}
	copied := list.DeepCopy()
	copied.Items[0].Spec.PeerASN = 99999

	if list.Items[0].Spec.PeerASN != 65000 {
		t.Errorf("original list item mutated via copy")
	}
}

// TestBGPPeerPeerASNFieldName verifies the JSON key is "peerASN".
func TestBGPPeerPeerASNFieldName(t *testing.T) {
	orig := newTestPeer()
	data, err := json.Marshal(orig.Spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	raw, ok := m["peerASN"]
	if !ok {
		t.Fatal("expected JSON key \"peerASN\" not found")
	}
	val, ok := raw.(float64)
	if !ok {
		t.Fatalf("expected peerASN to be a number, got %T", raw)
	}
	if int64(val) != orig.Spec.PeerASN {
		t.Errorf("peerASN value: got %v, want %d", val, orig.Spec.PeerASN)
	}
}

// TestBGPPeerLargePeerASN verifies that 4-byte ASNs (values above signed int32 max)
// survive JSON round-trip correctly. This is the regression test for the
// format: int32 / maximum: 4294967295 schema bug.
func TestBGPPeerLargePeerASN(t *testing.T) {
	// Max 4-byte ASN — the boundary of the uint32 range.
	const maxASN = ^uint32(0) // 4294967295

	peer := &BGPPeer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPPeer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "large-asn-peer"},
		Spec: BGPPeerSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "test-router"},
			},
			PeerASN: int64(maxASN),
			Address: "10.0.0.2",
			AddressFamilies: []AddressFamily{
				{AFI: AFIIPv4, SAFI: SAFIUnicast},
			},
		},
	}

	data, err := json.Marshal(peer)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPPeer
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.PeerASN != int64(maxASN) {
		t.Errorf("PeerASN after round-trip: got %d, want %d", got.Spec.PeerASN, maxASN)
	}
}

// TestBGPPeerPeerASNAboveSignedInt32Max verifies that values > 2^31-1
// (the signed int32 ceiling) are handled correctly.
func TestBGPPeerPeerASNAboveSignedInt32Max(t *testing.T) {
	// 2^31 = 2147483648 — the first value beyond signed int32 range.
	const aboveSignedMax int64 = 2147483648

	peer := &BGPPeer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPPeer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "above-int32-peer"},
		Spec: BGPPeerSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "test-router"},
			},
			PeerASN: aboveSignedMax,
			Address: "10.0.0.2",
			AddressFamilies: []AddressFamily{
				{AFI: AFIIPv4, SAFI: SAFIUnicast},
			},
		},
	}

	data, err := json.Marshal(peer)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPPeer
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Spec.PeerASN != aboveSignedMax {
		t.Errorf("PeerASN after round-trip: got %d, want %d", got.Spec.PeerASN, aboveSignedMax)
	}
}
