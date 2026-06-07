package gobgp

import (
	"context"
	"testing"

	gobgpapi "github.com/osrg/gobgp/v4/api"
	"google.golang.org/grpc"

	"go.miloapis.com/bgp/internal/provider"
)

// fakeGoBgpClient implements gobgpapi.GoBgpServiceClient for unit tests.
// Only GetBgp, StopBgp, and StartBgp have real logic; all other methods
// return nil, nil so that ConfigureSpeaker can be tested without a live daemon.
type fakeGoBgpClient struct {
	getBgpFn    func(context.Context, *gobgpapi.GetBgpRequest, ...grpc.CallOption) (*gobgpapi.GetBgpResponse, error)
	stopBgpN    int
	startBgpReq *gobgpapi.StartBgpRequest
}

func (f *fakeGoBgpClient) GetBgp(ctx context.Context, in *gobgpapi.GetBgpRequest, opts ...grpc.CallOption) (*gobgpapi.GetBgpResponse, error) {
	return f.getBgpFn(ctx, in, opts...)
}
func (f *fakeGoBgpClient) StopBgp(_ context.Context, _ *gobgpapi.StopBgpRequest, _ ...grpc.CallOption) (*gobgpapi.StopBgpResponse, error) {
	f.stopBgpN++
	return nil, nil
}
func (f *fakeGoBgpClient) StartBgp(_ context.Context, in *gobgpapi.StartBgpRequest, _ ...grpc.CallOption) (*gobgpapi.StartBgpResponse, error) {
	f.startBgpReq = in
	return nil, nil
}

// Stub implementations — not exercised by ConfigureSpeaker tests.
func (f *fakeGoBgpClient) WatchEvent(_ context.Context, _ *gobgpapi.WatchEventRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.WatchEventResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddPeer(_ context.Context, _ *gobgpapi.AddPeerRequest, _ ...grpc.CallOption) (*gobgpapi.AddPeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeletePeer(_ context.Context, _ *gobgpapi.DeletePeerRequest, _ ...grpc.CallOption) (*gobgpapi.DeletePeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListPeer(_ context.Context, _ *gobgpapi.ListPeerRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListPeerResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) UpdatePeer(_ context.Context, _ *gobgpapi.UpdatePeerRequest, _ ...grpc.CallOption) (*gobgpapi.UpdatePeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ResetPeer(_ context.Context, _ *gobgpapi.ResetPeerRequest, _ ...grpc.CallOption) (*gobgpapi.ResetPeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ShutdownPeer(_ context.Context, _ *gobgpapi.ShutdownPeerRequest, _ ...grpc.CallOption) (*gobgpapi.ShutdownPeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) EnablePeer(_ context.Context, _ *gobgpapi.EnablePeerRequest, _ ...grpc.CallOption) (*gobgpapi.EnablePeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DisablePeer(_ context.Context, _ *gobgpapi.DisablePeerRequest, _ ...grpc.CallOption) (*gobgpapi.DisablePeerResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddPeerGroup(_ context.Context, _ *gobgpapi.AddPeerGroupRequest, _ ...grpc.CallOption) (*gobgpapi.AddPeerGroupResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeletePeerGroup(_ context.Context, _ *gobgpapi.DeletePeerGroupRequest, _ ...grpc.CallOption) (*gobgpapi.DeletePeerGroupResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListPeerGroup(_ context.Context, _ *gobgpapi.ListPeerGroupRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListPeerGroupResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) UpdatePeerGroup(_ context.Context, _ *gobgpapi.UpdatePeerGroupRequest, _ ...grpc.CallOption) (*gobgpapi.UpdatePeerGroupResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddDynamicNeighbor(_ context.Context, _ *gobgpapi.AddDynamicNeighborRequest, _ ...grpc.CallOption) (*gobgpapi.AddDynamicNeighborResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListDynamicNeighbor(_ context.Context, _ *gobgpapi.ListDynamicNeighborRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListDynamicNeighborResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeleteDynamicNeighbor(_ context.Context, _ *gobgpapi.DeleteDynamicNeighborRequest, _ ...grpc.CallOption) (*gobgpapi.DeleteDynamicNeighborResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddPath(_ context.Context, _ *gobgpapi.AddPathRequest, _ ...grpc.CallOption) (*gobgpapi.AddPathResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeletePath(_ context.Context, _ *gobgpapi.DeletePathRequest, _ ...grpc.CallOption) (*gobgpapi.DeletePathResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListPath(_ context.Context, _ *gobgpapi.ListPathRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListPathResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddPathStream(_ context.Context, _ ...grpc.CallOption) (grpc.ClientStreamingClient[gobgpapi.AddPathStreamRequest, gobgpapi.AddPathStreamResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) GetTable(_ context.Context, _ *gobgpapi.GetTableRequest, _ ...grpc.CallOption) (*gobgpapi.GetTableResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddVrf(_ context.Context, _ *gobgpapi.AddVrfRequest, _ ...grpc.CallOption) (*gobgpapi.AddVrfResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeleteVrf(_ context.Context, _ *gobgpapi.DeleteVrfRequest, _ ...grpc.CallOption) (*gobgpapi.DeleteVrfResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListVrf(_ context.Context, _ *gobgpapi.ListVrfRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListVrfResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddPolicy(_ context.Context, _ *gobgpapi.AddPolicyRequest, _ ...grpc.CallOption) (*gobgpapi.AddPolicyResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeletePolicy(_ context.Context, _ *gobgpapi.DeletePolicyRequest, _ ...grpc.CallOption) (*gobgpapi.DeletePolicyResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListPolicy(_ context.Context, _ *gobgpapi.ListPolicyRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListPolicyResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) SetPolicies(_ context.Context, _ *gobgpapi.SetPoliciesRequest, _ ...grpc.CallOption) (*gobgpapi.SetPoliciesResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddDefinedSet(_ context.Context, _ *gobgpapi.AddDefinedSetRequest, _ ...grpc.CallOption) (*gobgpapi.AddDefinedSetResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeleteDefinedSet(_ context.Context, _ *gobgpapi.DeleteDefinedSetRequest, _ ...grpc.CallOption) (*gobgpapi.DeleteDefinedSetResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListDefinedSet(_ context.Context, _ *gobgpapi.ListDefinedSetRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListDefinedSetResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddStatement(_ context.Context, _ *gobgpapi.AddStatementRequest, _ ...grpc.CallOption) (*gobgpapi.AddStatementResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeleteStatement(_ context.Context, _ *gobgpapi.DeleteStatementRequest, _ ...grpc.CallOption) (*gobgpapi.DeleteStatementResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListStatement(_ context.Context, _ *gobgpapi.ListStatementRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListStatementResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddPolicyAssignment(_ context.Context, _ *gobgpapi.AddPolicyAssignmentRequest, _ ...grpc.CallOption) (*gobgpapi.AddPolicyAssignmentResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeletePolicyAssignment(_ context.Context, _ *gobgpapi.DeletePolicyAssignmentRequest, _ ...grpc.CallOption) (*gobgpapi.DeletePolicyAssignmentResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListPolicyAssignment(_ context.Context, _ *gobgpapi.ListPolicyAssignmentRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListPolicyAssignmentResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) SetPolicyAssignment(_ context.Context, _ *gobgpapi.SetPolicyAssignmentRequest, _ ...grpc.CallOption) (*gobgpapi.SetPolicyAssignmentResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddRpki(_ context.Context, _ *gobgpapi.AddRpkiRequest, _ ...grpc.CallOption) (*gobgpapi.AddRpkiResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeleteRpki(_ context.Context, _ *gobgpapi.DeleteRpkiRequest, _ ...grpc.CallOption) (*gobgpapi.DeleteRpkiResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListRpki(_ context.Context, _ *gobgpapi.ListRpkiRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListRpkiResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) EnableRpki(_ context.Context, _ *gobgpapi.EnableRpkiRequest, _ ...grpc.CallOption) (*gobgpapi.EnableRpkiResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DisableRpki(_ context.Context, _ *gobgpapi.DisableRpkiRequest, _ ...grpc.CallOption) (*gobgpapi.DisableRpkiResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ResetRpki(_ context.Context, _ *gobgpapi.ResetRpkiRequest, _ ...grpc.CallOption) (*gobgpapi.ResetRpkiResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListRpkiTable(_ context.Context, _ *gobgpapi.ListRpkiTableRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListRpkiTableResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) EnableZebra(_ context.Context, _ *gobgpapi.EnableZebraRequest, _ ...grpc.CallOption) (*gobgpapi.EnableZebraResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) EnableMrt(_ context.Context, _ *gobgpapi.EnableMrtRequest, _ ...grpc.CallOption) (*gobgpapi.EnableMrtResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DisableMrt(_ context.Context, _ *gobgpapi.DisableMrtRequest, _ ...grpc.CallOption) (*gobgpapi.DisableMrtResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) AddBmp(_ context.Context, _ *gobgpapi.AddBmpRequest, _ ...grpc.CallOption) (*gobgpapi.AddBmpResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) DeleteBmp(_ context.Context, _ *gobgpapi.DeleteBmpRequest, _ ...grpc.CallOption) (*gobgpapi.DeleteBmpResponse, error) {
	return nil, nil
}
func (f *fakeGoBgpClient) ListBmp(_ context.Context, _ *gobgpapi.ListBmpRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gobgpapi.ListBmpResponse], error) {
	return nil, nil
}
func (f *fakeGoBgpClient) SetLogLevel(_ context.Context, _ *gobgpapi.SetLogLevelRequest, _ ...grpc.CallOption) (*gobgpapi.SetLogLevelResponse, error) {
	return nil, nil
}

func TestAfiSafiFromStrings(t *testing.T) {
	tests := []struct {
		afi      string
		safi     string
		wantAfi  gobgpapi.Family_Afi
		wantSafi gobgpapi.Family_Safi
	}{
		{"IPv4", "Unicast", gobgpapi.Family_AFI_IP, gobgpapi.Family_SAFI_UNICAST},
		{"IPv6", "Unicast", gobgpapi.Family_AFI_IP6, gobgpapi.Family_SAFI_UNICAST},
		{"IPv4", "VPNUnicast", gobgpapi.Family_AFI_IP, gobgpapi.Family_SAFI_MPLS_VPN},
		{"IPv6", "VPNUnicast", gobgpapi.Family_AFI_IP6, gobgpapi.Family_SAFI_MPLS_VPN},
		// Unknown AFI falls back to IPv6 (default branch).
		{"unknown", "Unicast", gobgpapi.Family_AFI_IP6, gobgpapi.Family_SAFI_UNICAST},
		// Unknown SAFI falls back to Unicast (default branch).
		{"IPv4", "unknown", gobgpapi.Family_AFI_IP, gobgpapi.Family_SAFI_UNICAST},
	}

	for _, tc := range tests {
		t.Run(tc.afi+"/"+tc.safi, func(t *testing.T) {
			gotAfi, gotSafi := afiSafiFromStrings(tc.afi, tc.safi)
			if gotAfi != tc.wantAfi {
				t.Errorf("AFI = %v, want %v", gotAfi, tc.wantAfi)
			}
			if gotSafi != tc.wantSafi {
				t.Errorf("SAFI = %v, want %v", gotSafi, tc.wantSafi)
			}
		})
	}
}

func TestBuildAfiSafis(t *testing.T) {
	t.Run("nil input defaults to IPv6 unicast", func(t *testing.T) {
		result := buildAfiSafis(nil)
		if len(result) != 1 {
			t.Fatalf("len = %d, want 1", len(result))
		}
		cfg := result[0].Config
		if cfg.Family.Afi != gobgpapi.Family_AFI_IP6 {
			t.Errorf("AFI = %v, want AFI_IP6", cfg.Family.Afi)
		}
		if cfg.Family.Safi != gobgpapi.Family_SAFI_UNICAST {
			t.Errorf("SAFI = %v, want SAFI_UNICAST", cfg.Family.Safi)
		}
		if !cfg.Enabled {
			t.Error("Enabled = false, want true")
		}
	})

	t.Run("empty slice defaults to IPv6 unicast", func(t *testing.T) {
		result := buildAfiSafis([]provider.AddressFamily{})
		if len(result) != 1 {
			t.Fatalf("len = %d, want 1", len(result))
		}
		if result[0].Config.Family.Afi != gobgpapi.Family_AFI_IP6 {
			t.Errorf("AFI = %v, want AFI_IP6", result[0].Config.Family.Afi)
		}
	})

	t.Run("explicit VPNUnicast families are passed through", func(t *testing.T) {
		families := []provider.AddressFamily{
			{AFI: "IPv4", SAFI: "VPNUnicast"},
			{AFI: "IPv6", SAFI: "VPNUnicast"},
		}
		result := buildAfiSafis(families)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Config.Family.Afi != gobgpapi.Family_AFI_IP ||
			result[0].Config.Family.Safi != gobgpapi.Family_SAFI_MPLS_VPN {
			t.Errorf("[0] family = %v/%v, want AFI_IP/SAFI_MPLS_VPN",
				result[0].Config.Family.Afi, result[0].Config.Family.Safi)
		}
		if result[1].Config.Family.Afi != gobgpapi.Family_AFI_IP6 ||
			result[1].Config.Family.Safi != gobgpapi.Family_SAFI_MPLS_VPN {
			t.Errorf("[1] family = %v/%v, want AFI_IP6/SAFI_MPLS_VPN",
				result[1].Config.Family.Afi, result[1].Config.Family.Safi)
		}
		for i, r := range result {
			if !r.Config.Enabled {
				t.Errorf("[%d] Enabled = false, want true", i)
			}
		}
	})
}

func TestBuildPeer(t *testing.T) {
	t.Run("basic mandatory fields", func(t *testing.T) {
		spec := provider.PeerSpec{
			Address:  "2001:db8::1",
			ASNumber: 64512,
			Timers:   provider.TimerConfig{HoldTime: 90, Keepalive: 30},
		}
		peer := buildPeer(spec)

		if peer.Conf.NeighborAddress != "2001:db8::1" {
			t.Errorf("NeighborAddress = %q, want %q", peer.Conf.NeighborAddress, "2001:db8::1")
		}
		if peer.Conf.PeerAsn != 64512 {
			t.Errorf("PeerAsn = %d, want 64512", peer.Conf.PeerAsn)
		}
		if peer.Timers.Config.HoldTime != 90 {
			t.Errorf("HoldTime = %d, want 90", peer.Timers.Config.HoldTime)
		}
		if peer.Timers.Config.KeepaliveInterval != 30 {
			t.Errorf("KeepaliveInterval = %d, want 30", peer.Timers.Config.KeepaliveInterval)
		}
		// Optional fields must be nil when not specified.
		if peer.EbgpMultihop != nil {
			t.Errorf("EbgpMultihop = %v, want nil", peer.EbgpMultihop)
		}
		if peer.TtlSecurity != nil {
			t.Errorf("TtlSecurity = %v, want nil", peer.TtlSecurity)
		}
		if peer.Conf.AllowOwnAsn != 0 {
			t.Errorf("AllowOwnAsn = %d, want 0", peer.Conf.AllowOwnAsn)
		}
	})

	t.Run("optional fields set when configured", func(t *testing.T) {
		multihop := int32(5)
		spec := provider.PeerSpec{
			Address:              "2001:db8::2",
			ASNumber:             65000,
			AllowAsIn:            2,
			Password:             "secret",
			EBGPMultihop:         &multihop,
			RouteReflectorClient: true,
			Passive:              true,
		}
		peer := buildPeer(spec)

		if peer.Conf.AllowOwnAsn != 2 {
			t.Errorf("AllowOwnAsn = %d, want 2", peer.Conf.AllowOwnAsn)
		}
		if peer.Conf.AuthPassword != "secret" {
			t.Errorf("AuthPassword = %q, want %q", peer.Conf.AuthPassword, "secret")
		}
		if peer.EbgpMultihop == nil || !peer.EbgpMultihop.Enabled || peer.EbgpMultihop.MultihopTtl != 5 {
			t.Errorf("EbgpMultihop = %v, want {Enabled:true MultihopTtl:5}", peer.EbgpMultihop)
		}
		if peer.RouteReflector == nil || !peer.RouteReflector.RouteReflectorClient {
			t.Errorf("RouteReflector = %v, want {RouteReflectorClient:true}", peer.RouteReflector)
		}
		if !peer.Transport.PassiveMode {
			t.Error("PassiveMode = false, want true")
		}
		if peer.TtlSecurity != nil {
			t.Errorf("TtlSecurity = %v, want nil (EBGPMultihop takes priority)", peer.TtlSecurity)
		}
	})

	t.Run("TTL security", func(t *testing.T) {
		ttl := int32(254)
		spec := provider.PeerSpec{
			Address:     "2001:db8::3",
			TTLSecurity: &ttl,
		}
		peer := buildPeer(spec)

		if peer.TtlSecurity == nil || !peer.TtlSecurity.Enabled || peer.TtlSecurity.TtlMin != 254 {
			t.Errorf("TtlSecurity = %v, want {Enabled:true TtlMin:254}", peer.TtlSecurity)
		}
		if peer.EbgpMultihop != nil {
			t.Errorf("EbgpMultihop = %v, want nil", peer.EbgpMultihop)
		}
	})
}

// TestAddPathInvalidCIDR verifies that addPath returns an error before making
// any gRPC call when the CIDR cannot be parsed. The Provider is intentionally
// constructed with a nil gRPC client — if the function ever reaches the client
// call the test will panic, making the early-return contract explicit.
func TestAddPathInvalidCIDR(t *testing.T) {
	tests := []struct {
		name string
		cidr string
	}{
		{"bare address without prefix length", "2001:db8::1"},
		{"not an address", "not-a-cidr"},
		{"empty string", ""},
	}

	p := &Provider{} // nil client is deliberate — error must occur before gRPC
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := p.addPath(context.Background(), tc.cidr, false)
			if err == nil {
				t.Fatalf("addPath(%q): expected error, got nil", tc.cidr)
			}
		})
	}
}

// TestConfigureSpeaker exercises the StopBgp/StartBgp guard logic, including the
// GoBGP v4 behaviour where an uninitialised daemon returns Asn=0 rather than
// a NotFound error.
func TestConfigureSpeaker(t *testing.T) {
	spec := provider.SpeakerSpec{
		ASNumber:   64512,
		RouterID:   "10.0.0.1",
		ListenPort: 1790,
	}

	t.Run("uninitialised (Asn=0): skips StopBgp, calls StartBgp", func(t *testing.T) {
		fake := &fakeGoBgpClient{
			getBgpFn: func(_ context.Context, _ *gobgpapi.GetBgpRequest, _ ...grpc.CallOption) (*gobgpapi.GetBgpResponse, error) {
				return &gobgpapi.GetBgpResponse{Global: &gobgpapi.Global{Asn: 0}}, nil
			},
		}
		p := &Provider{client: fake}

		restarted, err := p.ConfigureSpeaker(context.Background(), spec)
		if err != nil {
			t.Fatalf("ConfigureSpeaker: unexpected error: %v", err)
		}
		if !restarted {
			t.Error("restarted = false, want true")
		}
		if fake.stopBgpN != 0 {
			t.Errorf("StopBgp called %d time(s), want 0 — must not stop an uninitialised daemon", fake.stopBgpN)
		}
		if fake.startBgpReq == nil {
			t.Fatal("StartBgp not called")
		}
		g := fake.startBgpReq.Global
		if g.Asn != 64512 {
			t.Errorf("StartBgp Global.Asn = %d, want 64512", g.Asn)
		}
		if g.RouterId != "10.0.0.1" {
			t.Errorf("StartBgp Global.RouterId = %q, want %q", g.RouterId, "10.0.0.1")
		}
		if g.ListenPort != 1790 {
			t.Errorf("StartBgp Global.ListenPort = %d, want 1790", g.ListenPort)
		}
	})

	t.Run("running with matching config: no restart", func(t *testing.T) {
		fake := &fakeGoBgpClient{
			getBgpFn: func(_ context.Context, _ *gobgpapi.GetBgpRequest, _ ...grpc.CallOption) (*gobgpapi.GetBgpResponse, error) {
				return &gobgpapi.GetBgpResponse{Global: &gobgpapi.Global{
					Asn: 64512, RouterId: "10.0.0.1", ListenPort: 1790,
				}}, nil
			},
		}
		p := &Provider{client: fake}

		restarted, err := p.ConfigureSpeaker(context.Background(), spec)
		if err != nil {
			t.Fatalf("ConfigureSpeaker: unexpected error: %v", err)
		}
		if restarted {
			t.Error("restarted = true, want false (config unchanged)")
		}
		if fake.stopBgpN != 0 {
			t.Errorf("StopBgp called %d time(s), want 0", fake.stopBgpN)
		}
		if fake.startBgpReq != nil {
			t.Error("StartBgp called, want not called")
		}
	})

	t.Run("running with mismatched ASN: stops then restarts", func(t *testing.T) {
		fake := &fakeGoBgpClient{
			getBgpFn: func(_ context.Context, _ *gobgpapi.GetBgpRequest, _ ...grpc.CallOption) (*gobgpapi.GetBgpResponse, error) {
				return &gobgpapi.GetBgpResponse{Global: &gobgpapi.Global{
					Asn: 65000, RouterId: "10.0.0.1", ListenPort: 1790,
				}}, nil
			},
		}
		p := &Provider{client: fake}

		restarted, err := p.ConfigureSpeaker(context.Background(), spec)
		if err != nil {
			t.Fatalf("ConfigureSpeaker: unexpected error: %v", err)
		}
		if !restarted {
			t.Error("restarted = false, want true")
		}
		if fake.stopBgpN != 1 {
			t.Errorf("StopBgp called %d time(s), want 1", fake.stopBgpN)
		}
		if fake.startBgpReq == nil {
			t.Fatal("StartBgp not called")
		}
		if fake.startBgpReq.Global.Asn != 64512 {
			t.Errorf("StartBgp Global.Asn = %d, want 64512", fake.startBgpReq.Global.Asn)
		}
	})
}
