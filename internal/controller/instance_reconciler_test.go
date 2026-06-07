package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	"go.miloapis.com/cosmos/internal/provider"
)

// stubProvider records the most recent ConfigureSpeaker call for inspection.
type stubProvider struct {
	lastSpec provider.SpeakerSpec
}

func (s *stubProvider) ConfigureSpeaker(_ context.Context, spec provider.SpeakerSpec) (bool, error) {
	s.lastSpec = spec
	return false, nil
}
func (s *stubProvider) AddOrUpdatePeer(_ context.Context, _ provider.PeerSpec) error { return nil }
func (s *stubProvider) DeletePeer(_ context.Context, _ string) error                 { return nil }
func (s *stubProvider) AddOrUpdateAdvertisement(_ context.Context, _ provider.AdvertisementSpec) error {
	return nil
}
func (s *stubProvider) DeleteAdvertisement(_ context.Context, _ string) error            { return nil }
func (s *stubProvider) AddOrUpdatePolicy(_ context.Context, _ provider.PolicySpec) error { return nil }
func (s *stubProvider) DeletePolicy(_ context.Context, _ string) error                   { return nil }
func (s *stubProvider) Ready(_ context.Context) error                                    { return nil }
func (s *stubProvider) Capabilities(_ context.Context) (provider.CapabilitySet, error) {
	return provider.CapabilitySet{}, nil
}

// TestListenPortByDaemonType verifies that reconcileForProvider passes the
// correct listen port to ConfigureSpeaker for each daemon type:
//   - FRR   → 179   (standard BGP port; FRR owns the underlay)
//   - GoBGP → -1    (listener disabled; GoBGP only dials outbound to its RR)
//   - other → 0     (switch default; zero value for int32)
func TestListenPortByDaemonType(t *testing.T) {
	tests := []struct {
		daemonType     string
		wantListenPort int32
	}{
		{daemonType: "FRR", wantListenPort: 179},
		{daemonType: "GoBGP", wantListenPort: -1},
		{daemonType: "unknown", wantListenPort: 0},
	}

	for _, tc := range tests {
		t.Run(tc.daemonType, func(t *testing.T) {
			stub := &stubProvider{}
			reg := provider.NewRegistry()
			reg.Set("test-provider", stub)

			instance := &bgpv1alpha1.BGPInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
				Spec: bgpv1alpha1.BGPInstanceSpec{
					ASNumber:       64512,
					RouterIDSource: "Manual",
					RouterID:       "10.0.0.1",
					ProviderSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"daemon": tc.daemonType},
					},
				},
			}
			bp := &providersv1alpha1.BGPProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-provider",
					Labels: map[string]string{"daemon": tc.daemonType},
				},
				Spec: providersv1alpha1.BGPProviderSpec{Type: tc.daemonType},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(instance, bp).
				WithStatusSubresource(instance).
				Build()

			r := &InstanceReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Registry: reg,
			}

			if err := r.reconcileForProvider(context.Background(), instance, bp); err != nil {
				t.Fatalf("reconcileForProvider: %v", err)
			}

			if stub.lastSpec.ListenPort != tc.wantListenPort {
				t.Errorf("daemon %s: ListenPort = %d, want %d",
					tc.daemonType, stub.lastSpec.ListenPort, tc.wantListenPort)
			}
		})
	}
}

// TestSpeakerSpecPropagation verifies that reconcileForProvider propagates AS
// number and router ID from the BGPInstance spec to ConfigureSpeaker unchanged.
func TestSpeakerSpecPropagation(t *testing.T) {
	stub := &stubProvider{}
	reg := provider.NewRegistry()
	reg.Set("test-provider", stub)

	instance := &bgpv1alpha1.BGPInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
		Spec: bgpv1alpha1.BGPInstanceSpec{
			ASNumber:       65001,
			RouterIDSource: "Manual",
			RouterID:       "192.0.2.1",
			ProviderSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"type": "frr"},
			},
		},
	}
	bp := &providersv1alpha1.BGPProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-provider",
			Labels: map[string]string{"type": "frr"},
		},
		Spec: providersv1alpha1.BGPProviderSpec{Type: "FRR"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance, bp).
		WithStatusSubresource(instance).
		Build()

	r := &InstanceReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Registry: reg,
	}

	if err := r.reconcileForProvider(context.Background(), instance, bp); err != nil {
		t.Fatalf("reconcileForProvider: %v", err)
	}

	if stub.lastSpec.ASNumber != 65001 {
		t.Errorf("ASNumber = %d, want 65001", stub.lastSpec.ASNumber)
	}
	if stub.lastSpec.RouterID != "192.0.2.1" {
		t.Errorf("RouterID = %q, want %q", stub.lastSpec.RouterID, "192.0.2.1")
	}
}
