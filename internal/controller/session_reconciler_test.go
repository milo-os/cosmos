package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
)

// TestBuildBGPPeerPreservesEVPN verifies that buildBGPPeer passes the L2VPN/EVPN
// address family from the BGPSession spec through to the generated BGPPeer unchanged.
func TestBuildBGPPeerPreservesEVPN(t *testing.T) {
	r := &SessionReconciler{}

	session := &bgpv1alpha1.BGPSession{
		ObjectMeta: metav1.ObjectMeta{Name: "evpn-session"},
		Spec: bgpv1alpha1.BGPSessionSpec{
			FromInstanceRef: "overlay",
			AddressFamilies: []bgpv1alpha1.AddressFamily{
				{AFI: bgpv1alpha1.AFIL2VPN, SAFI: bgpv1alpha1.SAFIEVPN},
			},
		},
	}
	bp := &providersv1alpha1.BGPProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
	}
	toPeer := bgpv1alpha1.SessionPeer{
		Address:  "2001:db8::1",
		ASNumber: 65001,
	}

	peer := r.buildBGPPeer(session, bp, toPeer)

	if len(peer.Spec.AddressFamilies) != 1 {
		t.Fatalf("AddressFamilies len = %d, want 1", len(peer.Spec.AddressFamilies))
	}
	af := peer.Spec.AddressFamilies[0]
	if af.AFI != bgpv1alpha1.AFIL2VPN {
		t.Errorf("AFI = %q, want %q", af.AFI, bgpv1alpha1.AFIL2VPN)
	}
	if af.SAFI != bgpv1alpha1.SAFIEVPN {
		t.Errorf("SAFI = %q, want %q", af.SAFI, bgpv1alpha1.SAFIEVPN)
	}
}

// TestBuildBGPPeerPreservesMultipleFamilies verifies that a mixed address family
// list (VPNUnicast + EVPN) is preserved in full on the generated BGPPeer.
func TestBuildBGPPeerPreservesMultipleFamilies(t *testing.T) {
	r := &SessionReconciler{}

	session := &bgpv1alpha1.BGPSession{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed-session"},
		Spec: bgpv1alpha1.BGPSessionSpec{
			FromInstanceRef: "overlay",
			AddressFamilies: []bgpv1alpha1.AddressFamily{
				{AFI: bgpv1alpha1.AFIIPv6, SAFI: bgpv1alpha1.SAFIVPNUnicast},
				{AFI: bgpv1alpha1.AFIL2VPN, SAFI: bgpv1alpha1.SAFIEVPN},
			},
		},
	}
	bp := &providersv1alpha1.BGPProvider{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	toPeer := bgpv1alpha1.SessionPeer{Address: "2001:db8::1", ASNumber: 65001}

	peer := r.buildBGPPeer(session, bp, toPeer)

	if len(peer.Spec.AddressFamilies) != 2 {
		t.Fatalf("AddressFamilies len = %d, want 2", len(peer.Spec.AddressFamilies))
	}
	if peer.Spec.AddressFamilies[1].AFI != bgpv1alpha1.AFIL2VPN ||
		peer.Spec.AddressFamilies[1].SAFI != bgpv1alpha1.SAFIEVPN {
		t.Errorf("AddressFamilies[1] = {%s,%s}, want {L2VPN,EVPN}",
			peer.Spec.AddressFamilies[1].AFI, peer.Spec.AddressFamilies[1].SAFI)
	}
}

// TestReconcilePopInfraGeneratesEVPNPeer is an end-to-end controller test that
// runs reconcilePopInfra with an EVPN BGPSession and verifies that the generated
// BGPPeer carries the L2VPN/EVPN address family.
func TestReconcilePopInfraGeneratesEVPNPeer(t *testing.T) {
	session := &bgpv1alpha1.BGPSession{
		ObjectMeta: metav1.ObjectMeta{Name: "evpn-overlay"},
		Spec: bgpv1alpha1.BGPSessionSpec{
			FromProviderSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "vtep"},
			},
			FromInstanceRef: "overlay",
			ToPeers: []bgpv1alpha1.SessionPeer{
				{Address: "2001:db8::2", ASNumber: 65001, InstanceRef: "overlay"},
			},
			AddressFamilies: []bgpv1alpha1.AddressFamily{
				{AFI: bgpv1alpha1.AFIL2VPN, SAFI: bgpv1alpha1.SAFIEVPN},
			},
		},
	}
	bp := &providersv1alpha1.BGPProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"role": "vtep"},
		},
		Spec: providersv1alpha1.BGPProviderSpec{Type: "FRR"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(session, bp).
		WithStatusSubresource(session).
		Build()

	r := &SessionReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	if _, err := r.reconcilePopInfra(context.Background(), session); err != nil {
		t.Fatalf("reconcilePopInfra: %v", err)
	}

	var peerList bgpv1alpha1.BGPPeerList
	if err := fakeClient.List(context.Background(), &peerList); err != nil {
		t.Fatalf("list BGPPeers: %v", err)
	}
	if len(peerList.Items) != 1 {
		t.Fatalf("BGPPeer count = %d, want 1", len(peerList.Items))
	}
	peer := peerList.Items[0]
	if len(peer.Spec.AddressFamilies) != 1 {
		t.Fatalf("AddressFamilies len = %d, want 1", len(peer.Spec.AddressFamilies))
	}
	af := peer.Spec.AddressFamilies[0]
	if af.AFI != bgpv1alpha1.AFIL2VPN || af.SAFI != bgpv1alpha1.SAFIEVPN {
		t.Errorf("AddressFamilies[0] = {%s,%s}, want {L2VPN,EVPN}", af.AFI, af.SAFI)
	}
}
