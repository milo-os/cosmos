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

// TestConditionConstants verifies the condition type and idle reason constants.
func TestConditionConstants(t *testing.T) {
	if ConditionTypeReady != "Ready" {
		t.Errorf("ConditionTypeReady = %q, want %q", ConditionTypeReady, "Ready")
	}
	if ConditionTypeAccepted != "Accepted" {
		t.Errorf("ConditionTypeAccepted = %q, want %q", ConditionTypeAccepted, "Accepted")
	}
	if IdleReasonBackOff != "BackOff" {
		t.Errorf("IdleReasonBackOff = %q, want %q", IdleReasonBackOff, "BackOff")
	}
	if IdleReasonConnectionRefused != "ConnectionRefused" {
		t.Errorf("IdleReasonConnectionRefused = %q, want %q", IdleReasonConnectionRefused, "ConnectionRefused")
	}
	if IdleReasonHoldTimerExpired != "HoldTimerExpired" {
		t.Errorf("IdleReasonHoldTimerExpired = %q, want %q", IdleReasonHoldTimerExpired, "HoldTimerExpired")
	}
	if IdleReasonIdle != "Idle" {
		t.Errorf("IdleReasonIdle = %q, want %q", IdleReasonIdle, "Idle")
	}
}

// TestUpdatePeerConditions_Established verifies Ready=True for Established.
func TestUpdatePeerConditions_Established(t *testing.T) {
	var status BGPPeerStatus
	status.updatePeerConditions(BGPPeerStateEstablished, 42, "")

	c := findCondition(status.Conditions, ConditionTypeReady)
	if c == nil {
		t.Fatal("Ready condition not found")
	}
	if c.Status != metav1.ConditionTrue {
		t.Errorf("Ready.Status = %s, want True", c.Status)
	}
	if c.Reason != "Established" {
		t.Errorf("Ready.Reason = %q, want %q", c.Reason, "Established")
	}
	if c.ObservedGeneration != 42 {
		t.Errorf("ObservedGeneration = %d, want 42", c.ObservedGeneration)
	}
}

// TestUpdatePeerConditions_Intermediate verifies Ready=False with FSM Reason.
func TestUpdatePeerConditions_Intermediate(t *testing.T) {
	tests := []struct {
		state  BGPPeerState
		reason string
	}{
		{BGPPeerStateOpenConfirm, "OpenConfirm"},
		{BGPPeerStateOpenSent, "OpenSent"},
		{BGPPeerStateActive, "Active"},
		{BGPPeerStateConnect, "Connect"},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			var status BGPPeerStatus
			status.updatePeerConditions(tt.state, 1, "")

			c := findCondition(status.Conditions, ConditionTypeReady)
			if c == nil {
				t.Fatal("Ready condition not found")
			}
			if c.Status != metav1.ConditionFalse {
				t.Errorf("Ready.Status = %s, want False", c.Status)
			}
			if c.Reason != tt.reason {
				t.Errorf("Ready.Reason = %q, want %q", c.Reason, tt.reason)
			}
		})
	}
}

// TestUpdatePeerConditions_Idle verifies Ready=False with caller-supplied idle reason.
func TestUpdatePeerConditions_Idle(t *testing.T) {
	tests := []struct {
		reason string
	}{
		{IdleReasonBackOff},
		{IdleReasonConnectionRefused},
		{IdleReasonHoldTimerExpired},
		{IdleReasonIdle},
		{"CustomReason"},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			var status BGPPeerStatus
			status.updatePeerConditions(BGPPeerStateIdle, 7, tt.reason)

			c := findCondition(status.Conditions, ConditionTypeReady)
			if c == nil {
				t.Fatal("Ready condition not found")
			}
			if c.Status != metav1.ConditionFalse {
				t.Errorf("Ready.Status = %s, want False", c.Status)
			}
			if c.Reason != tt.reason {
				t.Errorf("Ready.Reason = %q, want %q", c.Reason, tt.reason)
			}
		})
	}
}

// TestUpdatePeerConditions_Unknown verifies the default branch.
func TestUpdatePeerConditions_Unknown(t *testing.T) {
	var status BGPPeerStatus
	status.updatePeerConditions("NonExistent", 0, "")

	c := findCondition(status.Conditions, ConditionTypeReady)
	if c == nil {
		t.Fatal("Ready condition not found")
	}
	if c.Status != metav1.ConditionFalse {
		t.Errorf("Ready.Status = %s, want False", c.Status)
	}
	if c.Reason != "Unknown" {
		t.Errorf("Ready.Reason = %q, want %q", c.Reason, "Unknown")
	}
}

// TestUpdatePeerConditions_Idempotent verifies SetStatusCondition replaces previous Ready.
func TestUpdatePeerConditions_Idempotent(t *testing.T) {
	var status BGPPeerStatus

	// First update: Idle.
	status.updatePeerConditions(BGPPeerStateIdle, 1, IdleReasonIdle)
	if len(status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(status.Conditions))
	}

	// Second update: Established — should replace, not append.
	status.updatePeerConditions(BGPPeerStateEstablished, 2, "")
	if len(status.Conditions) != 1 {
		t.Fatalf("expected 1 condition after update, got %d", len(status.Conditions))
	}
	c := findCondition(status.Conditions, ConditionTypeReady)
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Error("Ready should be True after Established update")
	}
	if c.ObservedGeneration != 2 {
		t.Errorf("ObservedGeneration = %d, want 2", c.ObservedGeneration)
	}
}

// TestSetAcceptedCondition verifies the Accepted condition helper.
func TestSetAcceptedCondition(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		var status BGPPeerStatus
		status.SetAcceptedCondition(true, 5, "ConfigAccepted", "Peer config accepted by runtime.")

		c := findCondition(status.Conditions, ConditionTypeAccepted)
		if c == nil {
			t.Fatal("Accepted condition not found")
		}
		if c.Status != metav1.ConditionTrue {
			t.Errorf("Accepted.Status = %s, want True", c.Status)
		}
		if c.Reason != "ConfigAccepted" {
			t.Errorf("Accepted.Reason = %q, want %q", c.Reason, "ConfigAccepted")
		}
	})

	t.Run("rejected", func(t *testing.T) {
		var status BGPPeerStatus
		status.SetAcceptedCondition(false, 3, "InvalidPeerAddress", "address must be a valid IP")

		c := findCondition(status.Conditions, ConditionTypeAccepted)
		if c == nil {
			t.Fatal("Accepted condition not found")
		}
		if c.Status != metav1.ConditionFalse {
			t.Errorf("Accepted.Status = %s, want False", c.Status)
		}
	})

	t.Run("toggles", func(t *testing.T) {
		var status BGPPeerStatus
		// Accept first.
		status.SetAcceptedCondition(true, 1, "Accepted", "ok")
		c := findCondition(status.Conditions, ConditionTypeAccepted)
		if c == nil || c.Status != metav1.ConditionTrue {
			t.Error("expected True after first call")
		}
		// Then reject — should flip.
		status.SetAcceptedCondition(false, 2, "Rejected", "bad")
		c = findCondition(status.Conditions, ConditionTypeAccepted)
		if c == nil || c.Status != metav1.ConditionFalse {
			t.Error("expected False after second call")
		}
		if c.ObservedGeneration != 2 {
			t.Errorf("ObservedGeneration = %d, want 2", c.ObservedGeneration)
		}
	})
}

// TestUpdatePeerConditions_CoexistWithAccepted verifies Ready and Accepted
// conditions can coexist in the same Conditions slice.
func TestUpdatePeerConditions_CoexistWithAccepted(t *testing.T) {
	var status BGPPeerStatus
	status.SetAcceptedCondition(true, 1, "Accepted", "config accepted")
	status.updatePeerConditions(BGPPeerStateEstablished, 1, "")

	if len(status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(status.Conditions))
	}

	ready := findCondition(status.Conditions, ConditionTypeReady)
	accepted := findCondition(status.Conditions, ConditionTypeAccepted)
	if ready == nil || accepted == nil {
		t.Error("both Ready and Accepted conditions should be present")
	}
}

// findCondition returns the condition of the given type, or nil if not found.
func findCondition(conds []metav1.Condition, typ string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == typ {
			return &conds[i]
		}
	}
	return nil
}

// TestBGPPeerNewFieldsJSONRoundTrip verifies that multiSession, routeMapIn,
// and routeMapOut survive JSON serialization and deserialization.
func TestBGPPeerNewFieldsJSONRoundTrip(t *testing.T) {
	peer := &BGPPeer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPPeer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "new-fields-peer"},
		Spec: BGPPeerSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "test-router"},
			},
			PeerASN:      65000,
			Address:      "10.0.0.2",
			MultiSession: true,
			RouteMapIn:   "evpn-import",
			RouteMapOut:  "evpn-export",
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

	if got.Spec.MultiSession != peer.Spec.MultiSession {
		t.Errorf("MultiSession: got %v, want %v", got.Spec.MultiSession, peer.Spec.MultiSession)
	}
	if got.Spec.RouteMapIn != peer.Spec.RouteMapIn {
		t.Errorf("RouteMapIn: got %q, want %q", got.Spec.RouteMapIn, peer.Spec.RouteMapIn)
	}
	if got.Spec.RouteMapOut != peer.Spec.RouteMapOut {
		t.Errorf("RouteMapOut: got %q, want %q", got.Spec.RouteMapOut, peer.Spec.RouteMapOut)
	}
}

// TestBGPPeerNewFieldsJSONKeys verifies the JSON key names for the new fields
// are present when set to non-zero values.
func TestBGPPeerNewFieldsJSONKeys(t *testing.T) {
	peer := &BGPPeer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPPeer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "keys-peer"},
		Spec: BGPPeerSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "test-router"},
			},
			PeerASN:      65000,
			Address:      "10.0.0.2",
			MultiSession: true,
			RouteMapIn:   "evpn-import",
			RouteMapOut:  "evpn-export",
			AddressFamilies: []AddressFamily{
				{AFI: AFIIPv4, SAFI: SAFIUnicast},
			},
		},
	}

	data, err := json.Marshal(peer.Spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{"multiSession", "routeMapIn", "routeMapOut"} {
		raw, ok := m[key]
		if !ok {
			t.Fatalf("expected JSON key %q not found", key)
		}
		_ = raw
	}
}

// TestBGPPeerNewFieldsDeepCopy verifies DeepCopy handles the new fields correctly.
func TestBGPPeerNewFieldsDeepCopy(t *testing.T) {
	peer := &BGPPeer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPPeer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "copy-new-fields-peer"},
		Spec: BGPPeerSpec{
			RouterTarget: RouterTarget{
				RouterRef: &RouterRef{Name: "test-router"},
			},
			PeerASN:      65000,
			Address:      "10.0.0.2",
			MultiSession: true,
			RouteMapIn:   "evpn-import",
			RouteMapOut:  "evpn-export",
			AddressFamilies: []AddressFamily{
				{AFI: AFIIPv4, SAFI: SAFIUnicast},
			},
		},
	}

	copied := peer.DeepCopy()

	copied.Spec.MultiSession = false
	copied.Spec.RouteMapIn = "mutated"
	copied.Spec.RouteMapOut = "mutated"

	if peer.Spec.MultiSession != true {
		t.Errorf("MultiSession mutated via copy: got %v, want true", peer.Spec.MultiSession)
	}
	if peer.Spec.RouteMapIn != "evpn-import" {
		t.Errorf("RouteMapIn mutated via copy: got %q, want %q", peer.Spec.RouteMapIn, "evpn-import")
	}
	if peer.Spec.RouteMapOut != "evpn-export" {
		t.Errorf("RouteMapOut mutated via copy: got %q, want %q", peer.Spec.RouteMapOut, "evpn-export")
	}
}

// TestBGPPeerStatusNewFieldsJSONRoundTrip verifies the new status fields
// survive JSON serialization and deserialization.
func TestBGPPeerStatusNewFieldsJSONRoundTrip(t *testing.T) {
	now := metav1.Now()
	status := BGPPeerStatus{
		ObservedGeneration:  5,
		SessionState:        BGPPeerStateEstablished,
		LastEstablishedTime: &now,
		LastStateChange:     &now,
		Uptime:              &metav1.Duration{Duration: 3600000000000}, // 1h
		MessagesSent:        1500,
		MessagesReceived:    1480,
		AFISAFIStats: []BGPPeerAFISAFIStats{
			{AFI: AFIIPv4, SAFI: SAFIUnicast, PrefixesReceived: 100, PrefixesAdvertised: 95},
			{AFI: AFIIPv6, SAFI: SAFIUnicast, PrefixesReceived: 50, PrefixesAdvertised: 48},
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got BGPPeerStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration: got %d, want 5", got.ObservedGeneration)
	}
	if got.SessionState != BGPPeerStateEstablished {
		t.Errorf("SessionState: got %q, want %q", got.SessionState, BGPPeerStateEstablished)
	}
	if got.LastStateChange == nil {
		t.Fatal("LastStateChange is nil")
	}
	if got.MessagesSent != 1500 {
		t.Errorf("MessagesSent: got %d, want 1500", got.MessagesSent)
	}
	if got.MessagesReceived != 1480 {
		t.Errorf("MessagesReceived: got %d, want 1480", got.MessagesReceived)
	}
	if len(got.AFISAFIStats) != 2 {
		t.Fatalf("AFISAFIStats count: got %d, want 2", len(got.AFISAFIStats))
	}
	if got.AFISAFIStats[0].PrefixesReceived != 100 {
		t.Errorf("AFISAFIStats[0].PrefixesReceived: got %d, want 100", got.AFISAFIStats[0].PrefixesReceived)
	}
	if got.AFISAFIStats[1].PrefixesAdvertised != 48 {
		t.Errorf("AFISAFIStats[1].PrefixesAdvertised: got %d, want 48", got.AFISAFIStats[1].PrefixesAdvertised)
	}
}

// TestBGPPeerStatusNewFieldsJSONKeys verifies the JSON key names for the new
// status fields are present when set to non-zero values.
func TestBGPPeerStatusNewFieldsJSONKeys(t *testing.T) {
	status := BGPPeerStatus{
		SessionState:     BGPPeerStateEstablished,
		LastStateChange:  &metav1.Time{},
		Uptime:           &metav1.Duration{},
		MessagesSent:     100,
		MessagesReceived: 200,
		AFISAFIStats: []BGPPeerAFISAFIStats{
			{AFI: AFIIPv4, SAFI: SAFIUnicast},
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

	for _, key := range []string{"lastStateChange", "uptime", "messagesSent", "messagesReceived", "afiSafiStats"} {
		raw, ok := m[key]
		if !ok {
			t.Fatalf("expected JSON key %q not found", key)
		}
		if key == "afiSafiStats" {
			arr, ok := raw.([]any)
			if !ok {
				t.Errorf("expected %q to be an array, got %T", key, raw)
			} else if len(arr) != 1 {
				t.Errorf("expected %q array length 1, got %d", key, len(arr))
			}
		}
	}
}

// TestBGPPeerStatusNewFieldsDeepCopy verifies DeepCopy handles the new status
// fields correctly.
func TestBGPPeerStatusNewFieldsDeepCopy(t *testing.T) {
	now := metav1.Now()
	orig := &BGPPeerStatus{
		ObservedGeneration: 1,
		SessionState:       BGPPeerStateEstablished,
		LastStateChange:    &now,
		Uptime:             &metav1.Duration{Duration: 3600000000000},
		MessagesSent:       1000,
		MessagesReceived:   999,
		AFISAFIStats: []BGPPeerAFISAFIStats{
			{AFI: AFIIPv4, SAFI: SAFIUnicast, PrefixesReceived: 50, PrefixesAdvertised: 45},
			{AFI: AFIIPv6, SAFI: SAFIUnicast, PrefixesReceived: 25, PrefixesAdvertised: 20},
		},
	}

	copied := orig.DeepCopy()

	copied.SessionState = BGPPeerStateIdle
	copied.MessagesSent = 9999
	copied.AFISAFIStats[0].PrefixesReceived = 0

	if orig.SessionState != BGPPeerStateEstablished {
		t.Errorf("SessionState mutated via copy: got %q, want %q", orig.SessionState, BGPPeerStateEstablished)
	}
	if orig.MessagesSent != 1000 {
		t.Errorf("MessagesSent mutated via copy: got %d, want 1000", orig.MessagesSent)
	}
	if orig.AFISAFIStats[0].PrefixesReceived != 50 {
		t.Errorf("AFISAFIStats[0].PrefixesReceived mutated via copy: got %d, want 50", orig.AFISAFIStats[0].PrefixesReceived)
	}
}
