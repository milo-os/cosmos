package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	"go.miloapis.com/cosmos/internal/provider"
)

// stubProvider records the most recent ConfigureInstance call for inspection.
type stubProvider struct {
	lastSpec provider.InstanceSpec
}

func (s *stubProvider) ConfigureInstance(_ context.Context, spec provider.InstanceSpec) (bool, error) {
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

// TestListenPortPassthrough verifies that reconcileForProvider passes spec.listenPort
// to ConfigureInstance unchanged, and falls back to 179 when the field is nil
// (objects created before the API default was introduced).
func TestListenPortPassthrough(t *testing.T) {
	p179 := int32(179)
	pNeg1 := int32(-1)
	p1790 := int32(1790)
	p1179 := int32(1179)

	tests := []struct {
		name           string
		listenPort     *int32
		wantListenPort int32
	}{
		{name: "standard port", listenPort: &p179, wantListenPort: 179},
		{name: "disabled (-1)", listenPort: &pNeg1, wantListenPort: -1},
		{name: "RR port", listenPort: &p1790, wantListenPort: 1790},
		{name: "custom port", listenPort: &p1179, wantListenPort: 1179},
		{name: "nil falls back to 179", listenPort: nil, wantListenPort: 179},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubProvider{}
			pool := provider.NewPool()
			pool.SetForTest("test-provider", stub)

			instance := &bgpv1alpha1.BGPInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
				Spec: bgpv1alpha1.BGPInstanceSpec{
					ASNumber:       64512,
					RouterIDSource: "Manual",
					RouterID:       "10.0.0.1",
					ListenPort:     tc.listenPort,
					ProviderSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"type": "test-agent"},
					},
				},
			}
			bp := &providersv1alpha1.BGPProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-provider",
					Labels: map[string]string{"type": "test-agent"},
				},
				Spec: providersv1alpha1.BGPProviderSpec{Type: "test-agent"},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(instance, bp).
				WithStatusSubresource(instance).
				Build()

			r := &InstanceReconciler{
				Client: fakeClient,
				Scheme: scheme,
				Pool:   pool,
			}

			if err := r.reconcileForProvider(context.Background(), instance, bp); err != nil {
				t.Fatalf("reconcileForProvider: %v", err)
			}

			if stub.lastSpec.ListenPort != tc.wantListenPort {
				t.Errorf("ListenPort = %d, want %d", stub.lastSpec.ListenPort, tc.wantListenPort)
			}
		})
	}
}

// TestListenPortExplicitOverride verifies that the listen port is always sourced
// from spec.listenPort regardless of agent type or RR config.
func TestListenPortExplicitOverride(t *testing.T) {
	rrSpec := &bgpv1alpha1.RouteReflectorConfig{ClusterID: "1.0.0.1"}
	port := int32(1179)

	tests := []struct {
		name           string
		daemonType     string
		routeReflector *bgpv1alpha1.RouteReflectorConfig
		listenPort     *int32
		wantListenPort int32
	}{
		{name: "worker agent", daemonType: "agent-a", listenPort: &port, wantListenPort: 1179},
		{name: "RR agent", daemonType: "agent-a", routeReflector: rrSpec, listenPort: &port, wantListenPort: 1179},
		{name: "other agent", daemonType: "agent-b", listenPort: &port, wantListenPort: 1179},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubProvider{}
			pool := provider.NewPool()
			pool.SetForTest("test-provider", stub)

			instance := &bgpv1alpha1.BGPInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
				Spec: bgpv1alpha1.BGPInstanceSpec{
					ASNumber:       64512,
					RouterIDSource: "Manual",
					RouterID:       "10.0.0.1",
					RouteReflector: tc.routeReflector,
					ListenPort:     tc.listenPort,
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
				Client: fakeClient,
				Scheme: scheme,
				Pool:   pool,
			}

			if err := r.reconcileForProvider(context.Background(), instance, bp); err != nil {
				t.Fatalf("reconcileForProvider: %v", err)
			}

			if stub.lastSpec.ListenPort != tc.wantListenPort {
				t.Errorf("ListenPort = %d, want %d", stub.lastSpec.ListenPort, tc.wantListenPort)
			}
		})
	}
}

// TestInstanceSpecPropagation verifies that reconcileForProvider propagates AS
// number and router ID from the BGPInstance spec to ConfigureInstance unchanged.
func TestInstanceSpecPropagation(t *testing.T) {
	stub := &stubProvider{}
	pool := provider.NewPool()
	pool.SetForTest("test-provider", stub)

	instance := &bgpv1alpha1.BGPInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
		Spec: bgpv1alpha1.BGPInstanceSpec{
			ASNumber:       65001,
			RouterIDSource: "Manual",
			RouterID:       "192.0.2.1",
			ProviderSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"type": "test-agent"},
			},
		},
	}
	bp := &providersv1alpha1.BGPProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-provider",
			Labels: map[string]string{"type": "test-agent"},
		},
		Spec: providersv1alpha1.BGPProviderSpec{Type: "test-agent"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance, bp).
		WithStatusSubresource(instance).
		Build()

	r := &InstanceReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Pool:   pool,
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

// TestResolveRouterIDAutoSource verifies that resolveRouterID prefers IPv6 when
// available and falls back to IPv4 for IPv4-only nodes (e.g. Kind without dual-stack).
func TestResolveRouterIDAutoSource(t *testing.T) {
	tests := []struct {
		name      string
		addresses []corev1.NodeAddress
		wantID    string
		wantErr   bool
	}{
		{
			name: "IPv6 only",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "fd00::1"},
			},
			wantID: "0.0.0.1",
		},
		{
			name: "IPv4 only fallback",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.5"},
			},
			wantID: "10.0.0.5",
		},
		{
			name: "dual-stack prefers IPv6",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.5"},
				{Type: corev1.NodeInternalIP, Address: "fd00::1"},
			},
			wantID: "0.0.0.1",
		},
		{
			name: "no InternalIP addresses",
			addresses: []corev1.NodeAddress{
				{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				Status:     corev1.NodeStatus{Addresses: tc.addresses},
			}
			bp := &providersv1alpha1.BGPProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-provider",
					Labels: map[string]string{LabelNode: "test-node"},
				},
				Spec: providersv1alpha1.BGPProviderSpec{Type: "test-agent"},
			}
			instance := &bgpv1alpha1.BGPInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance"},
				Spec: bgpv1alpha1.BGPInstanceSpec{
					ASNumber:       64512,
					RouterIDSource: "Auto",
					ProviderSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{LabelNode: "test-node"},
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(instance, bp, node).
				WithStatusSubresource(instance).
				Build()

			r := &InstanceReconciler{
				Client: fakeClient,
				Scheme: scheme,
				Pool:   provider.NewPool(),
			}

			gotID, err := r.resolveRouterID(context.Background(), instance, bp)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got routerID=%q", gotID)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRouterID: %v", err)
			}
			if gotID != tc.wantID {
				t.Errorf("routerID = %q, want %q", gotID, tc.wantID)
			}
		})
	}
}
