package frr

import (
	"testing"
)

func TestFrrAFI(t *testing.T) {
	tests := []struct {
		afi  string
		want string
	}{
		{"IPv4", "ipv4"},
		{"IPv6", "ipv6"},
		{"L2VPN", "l2vpn"},
		{"unknown", "ipv6"}, // default branch
	}
	for _, tc := range tests {
		t.Run(tc.afi, func(t *testing.T) {
			got := frrAFI(tc.afi)
			if got != tc.want {
				t.Errorf("frrAFI(%q) = %q, want %q", tc.afi, got, tc.want)
			}
		})
	}
}

func TestFrrSAFI(t *testing.T) {
	tests := []struct {
		safi string
		want string
	}{
		{"Unicast", "unicast"},
		{"VPNUnicast", "vpn"},
		{"EVPN", "evpn"},
		{"unknown", "unicast"}, // default branch
	}
	for _, tc := range tests {
		t.Run(tc.safi, func(t *testing.T) {
			got := frrSAFI(tc.safi)
			if got != tc.want {
				t.Errorf("frrSAFI(%q) = %q, want %q", tc.safi, got, tc.want)
			}
		})
	}
}

func TestFRRCapabilitiesContainEVPN(t *testing.T) {
	for _, af := range FRRCapabilities.AddressFamilies {
		if af.AFI == "L2VPN" && af.SAFI == "EVPN" {
			return
		}
	}
	t.Error("FRRCapabilities.AddressFamilies does not contain {L2VPN, EVPN}")
}
