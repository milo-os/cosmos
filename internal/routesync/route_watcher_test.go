package routesync

import (
	"net"
	"net/netip"
	"testing"
	"time"

	bgppkt "github.com/osrg/gobgp/v4/pkg/packet/bgp"
	"github.com/osrg/gobgp/v4/pkg/apiutil"

	gobgpapi "github.com/osrg/gobgp/v4/api"
)

// buildTestPath constructs a gobgpapi.Path for an IPv6 unicast prefix using the
// packet-layer bgppkt types and apiutil.NewPath. It mirrors the cosmos addPath
// helper so that extractPrefixAndNextHop can be tested with realistic paths
// without a live GoBGP daemon.
func buildTestPath(t *testing.T, prefix string, nextHop string, isWithdraw bool) *gobgpapi.Path {
	t.Helper()

	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		t.Fatalf("parse prefix %q: %v", prefix, err)
	}
	nlri, err := bgppkt.NewIPAddrPrefix(p.Masked())
	if err != nil {
		t.Fatalf("NewIPAddrPrefix %q: %v", prefix, err)
	}
	nh, err := netip.ParseAddr(nextHop)
	if err != nil {
		t.Fatalf("parse next-hop %q: %v", nextHop, err)
	}
	mpReach, err := bgppkt.NewPathAttributeMpReachNLRI(
		bgppkt.RF_IPv6_UC,
		[]bgppkt.PathNLRI{{NLRI: nlri}},
		nh,
	)
	if err != nil {
		t.Fatalf("NewPathAttributeMpReachNLRI: %v", err)
	}
	attrs := []bgppkt.PathAttributeInterface{
		bgppkt.NewPathAttributeOrigin(bgppkt.BGP_ORIGIN_ATTR_TYPE_IGP),
		mpReach,
	}
	path, err := apiutil.NewPath(bgppkt.RF_IPv6_UC, nlri, isWithdraw, attrs, time.Now())
	if err != nil {
		t.Fatalf("apiutil.NewPath: %v", err)
	}
	return path
}

// TestExtractPrefixAndNextHop validates that extractPrefixAndNextHop correctly
// parses IPv6 prefixes and converts the GoBGP v4 netip.Addr next-hop to net.IP.
// The netip.Addr → net.IP conversion (via AsSlice) is the key behavior change
// from the GoBGP v3→v4 upgrade.
func TestExtractPrefixAndNextHop(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		nextHop     string
		isWithdraw  bool
		wantPrefix  string
		wantNextHop net.IP
	}{
		{
			name:        "announce /32 with global unicast next-hop",
			prefix:      "2001:db8::/32",
			nextHop:     "2001:db8::1",
			wantPrefix:  "2001:db8::/32",
			wantNextHop: net.ParseIP("2001:db8::1"),
		},
		{
			name:        "announce /48 overlay prefix",
			prefix:      "fd12:3456:789a::/48",
			nextHop:     "fd12:3456::1",
			wantPrefix:  "fd12:3456:789a::/48",
			wantNextHop: net.ParseIP("fd12:3456::1"),
		},
		{
			name:        "withdraw path still carries next-hop from MpReachNLRI",
			prefix:      "2001:db8:1::/48",
			nextHop:     "2001:db8::2",
			isWithdraw:  true,
			wantPrefix:  "2001:db8:1::/48",
			wantNextHop: net.ParseIP("2001:db8::2"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := buildTestPath(t, tc.prefix, tc.nextHop, tc.isWithdraw)

			gotPrefix, gotNextHop, err := extractPrefixAndNextHop(path)
			if err != nil {
				t.Fatalf("extractPrefixAndNextHop: %v", err)
			}

			if gotPrefix.String() != tc.wantPrefix {
				t.Errorf("prefix = %q, want %q", gotPrefix, tc.wantPrefix)
			}
			if !gotNextHop.Equal(tc.wantNextHop) {
				t.Errorf("nextHop = %v, want %v", gotNextHop, tc.wantNextHop)
			}
		})
	}
}

// TestExtractPrefixAndNextHopNoNextHop asserts that a path carrying no
// next-hop attribute returns an error rather than a nil net.IP, which would
// silently program a black-hole route via netlink.
func TestExtractPrefixAndNextHopNoNextHop(t *testing.T) {
	p, _ := netip.ParsePrefix("2001:db8::/32")
	nlri, err := bgppkt.NewIPAddrPrefix(p.Masked())
	if err != nil {
		t.Fatalf("NewIPAddrPrefix: %v", err)
	}
	// Origin only — no MpReachNLRI or NextHop attribute.
	attrs := []bgppkt.PathAttributeInterface{
		bgppkt.NewPathAttributeOrigin(bgppkt.BGP_ORIGIN_ATTR_TYPE_IGP),
	}
	path, err := apiutil.NewPath(bgppkt.RF_IPv6_UC, nlri, false, attrs, time.Now())
	if err != nil {
		t.Fatalf("apiutil.NewPath: %v", err)
	}

	_, _, err = extractPrefixAndNextHop(path)
	if err == nil {
		t.Fatal("expected error for path with no next-hop, got nil")
	}
}
