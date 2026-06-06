package gobgp

import (
	"context"
	"testing"

	gobgpapi "github.com/osrg/gobgp/v4/api"

	"go.miloapis.com/bgp/internal/provider"
)

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
