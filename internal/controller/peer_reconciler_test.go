package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	"go.miloapis.com/cosmos/internal/provider"
)

// peerRecordingProvider records the most recent AddOrUpdatePeer call for inspection.
type peerRecordingProvider struct {
	lastPeerSpec provider.PeerSpec
}

func (p *peerRecordingProvider) ConfigureSpeaker(_ context.Context, _ provider.SpeakerSpec) (bool, error) {
	return false, nil
}
func (p *peerRecordingProvider) AddOrUpdatePeer(_ context.Context, spec provider.PeerSpec) error {
	p.lastPeerSpec = spec
	return nil
}
func (p *peerRecordingProvider) DeletePeer(_ context.Context, _ string) error { return nil }
func (p *peerRecordingProvider) AddOrUpdateAdvertisement(_ context.Context, _ provider.AdvertisementSpec) error {
	return nil
}
func (p *peerRecordingProvider) DeleteAdvertisement(_ context.Context, _ string) error { return nil }
func (p *peerRecordingProvider) AddOrUpdatePolicy(_ context.Context, _ provider.PolicySpec) error {
	return nil
}
func (p *peerRecordingProvider) DeletePolicy(_ context.Context, _ string) error { return nil }
func (p *peerRecordingProvider) Ready(_ context.Context) error                  { return nil }
func (p *peerRecordingProvider) Capabilities(_ context.Context) (provider.CapabilitySet, error) {
	return provider.CapabilitySet{}, nil
}

// TestPeerReconcilerEVPNAddressFamily is an end-to-end controller test that
// verifies L2VPN/EVPN address families flow from BGPPeer.spec through
// reconcileForProvider to the provider's AddOrUpdatePeer call.
func TestPeerReconcilerEVPNAddressFamily(t *testing.T) {
	instance := &bgpv1alpha1.BGPInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "overlay"},
		Spec: bgpv1alpha1.BGPInstanceSpec{
			ASNumber:       65001,
			RouterIDSource: "Manual",
			RouterID:       "10.0.0.1",
			ProviderSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "vtep"},
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
	bgpPeer := &bgpv1alpha1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{Name: "evpn-peer"},
		Spec: bgpv1alpha1.BGPPeerSpec{
			InstanceRef: "overlay",
			ProviderRef: "node-a",
			Address:     "2001:db8::2",
			ASNumber:    65001,
			AddressFamilies: []bgpv1alpha1.AddressFamily{
				{AFI: bgpv1alpha1.AFIL2VPN, SAFI: bgpv1alpha1.SAFIEVPN},
			},
		},
	}

	recording := &peerRecordingProvider{}
	pool := provider.NewPool()
	pool.SetForTest("node-a", recording)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance, bp, bgpPeer).
		WithStatusSubresource(bgpPeer).
		Build()

	r := &PeerReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Pool:   pool,
	}

	if err := r.reconcileForProvider(context.Background(), bgpPeer, instance, *bp); err != nil {
		t.Fatalf("reconcileForProvider: %v", err)
	}

	fams := recording.lastPeerSpec.Families
	if len(fams) != 1 {
		t.Fatalf("Families len = %d, want 1", len(fams))
	}
	if fams[0].AFI != bgpv1alpha1.AFIL2VPN {
		t.Errorf("Families[0].AFI = %q, want %q", fams[0].AFI, bgpv1alpha1.AFIL2VPN)
	}
	if fams[0].SAFI != bgpv1alpha1.SAFIEVPN {
		t.Errorf("Families[0].SAFI = %q, want %q", fams[0].SAFI, bgpv1alpha1.SAFIEVPN)
	}
}

// TestPeerReconcilerEVPNInheritedFromInstance verifies that when BGPPeer has no
// explicit address families, it inherits L2VPN/EVPN from the BGPInstance.
func TestPeerReconcilerEVPNInheritedFromInstance(t *testing.T) {
	instance := &bgpv1alpha1.BGPInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "overlay"},
		Spec: bgpv1alpha1.BGPInstanceSpec{
			ASNumber:       65001,
			RouterIDSource: "Manual",
			RouterID:       "10.0.0.1",
			ProviderSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "vtep"},
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
	// BGPPeer carries no addressFamilies — must inherit from instance.
	bgpPeer := &bgpv1alpha1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{Name: "evpn-peer-inherit"},
		Spec: bgpv1alpha1.BGPPeerSpec{
			InstanceRef: "overlay",
			ProviderRef: "node-a",
			Address:     "2001:db8::2",
			ASNumber:    65001,
		},
	}

	recording := &peerRecordingProvider{}
	pool := provider.NewPool()
	pool.SetForTest("node-a", recording)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance, bp, bgpPeer).
		WithStatusSubresource(bgpPeer).
		Build()

	r := &PeerReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Pool:   pool,
	}

	if err := r.reconcileForProvider(context.Background(), bgpPeer, instance, *bp); err != nil {
		t.Fatalf("reconcileForProvider: %v", err)
	}

	fams := recording.lastPeerSpec.Families
	if len(fams) != 1 {
		t.Fatalf("Families len = %d, want 1 (inherited from instance)", len(fams))
	}
	if fams[0].AFI != bgpv1alpha1.AFIL2VPN || fams[0].SAFI != bgpv1alpha1.SAFIEVPN {
		t.Errorf("Families[0] = {%s,%s}, want {L2VPN,EVPN}", fams[0].AFI, fams[0].SAFI)
	}
}
