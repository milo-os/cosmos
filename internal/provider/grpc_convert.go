package provider

import (
	providerv1alpha1 "go.miloapis.com/cosmos/api/proto/bgp/provider/v1alpha1"
)

func speakerSpecToProto(spec SpeakerSpec) *providerv1alpha1.ConfigureSpeakerRequest {
	families := make([]*providerv1alpha1.AddressFamily, 0, len(spec.Families))
	for _, af := range spec.Families {
		families = append(families, &providerv1alpha1.AddressFamily{Afi: af.AFI, Safi: af.SAFI})
	}

	s := &providerv1alpha1.SpeakerSpec{
		AsNumber:   spec.ASNumber,
		RouterId:   spec.RouterID,
		ListenPort: spec.ListenPort,
		Families:   families,
		Timers: &providerv1alpha1.TimerConfig{
			HoldTime:  spec.Timers.HoldTime,
			Keepalive: spec.Timers.Keepalive,
		},
		BestPath: &providerv1alpha1.BestPathConfig{
			AlwaysCompareMed: spec.BestPath.AlwaysCompareMed,
			DeterministicMed: spec.BestPath.DeterministicMed,
			CompareRouterId:  spec.BestPath.CompareRouterID,
		},
	}
	if spec.RouteReflector != nil {
		s.RouteReflector = &providerv1alpha1.RouteReflectorConfig{
			ClusterId: spec.RouteReflector.ClusterID,
		}
	}
	return &providerv1alpha1.ConfigureSpeakerRequest{Spec: s}
}

func peerSpecToProto(peer PeerSpec) *providerv1alpha1.PeerSpec {
	families := make([]*providerv1alpha1.AddressFamily, 0, len(peer.Families))
	for _, af := range peer.Families {
		families = append(families, &providerv1alpha1.AddressFamily{Afi: af.AFI, Safi: af.SAFI})
	}

	p := &providerv1alpha1.PeerSpec{
		Address:              peer.Address,
		AsNumber:             peer.ASNumber,
		Families:             families,
		Timers:               &providerv1alpha1.TimerConfig{HoldTime: peer.Timers.HoldTime, Keepalive: peer.Timers.Keepalive},
		AllowAsIn:            peer.AllowAsIn,
		RouteReflectorClient: peer.RouteReflectorClient,
		Passive:              peer.Passive,
		Password:             peer.Password,
		RemotePort:           peer.RemotePort,
	}
	if peer.EBGPMultihop != nil {
		v := *peer.EBGPMultihop
		p.EbgpMultihop = &v
	}
	if peer.TTLSecurity != nil {
		v := *peer.TTLSecurity
		p.TtlSecurity = &v
	}
	return p
}

func policySpecToProto(pol PolicySpec) *providerv1alpha1.PolicySpec {
	return &providerv1alpha1.PolicySpec{
		Name:             pol.Name,
		Priority:         pol.Priority,
		ImportStatements: statementsToProto(pol.ImportStatements),
		ExportStatements: statementsToProto(pol.ExportStatements),
	}
}

func statementsToProto(stmts []PolicyStatement) []*providerv1alpha1.PolicyStatement {
	out := make([]*providerv1alpha1.PolicyStatement, 0, len(stmts))
	for _, s := range stmts {
		ps := &providerv1alpha1.PolicyStatement{
			Name:    s.Name,
			Actions: actionsToProto(s.Actions),
		}
		if s.Conditions != nil {
			ps.Conditions = &providerv1alpha1.PolicyConditions{
				PrefixSets:   s.Conditions.PrefixSets,
				CommunitySet: s.Conditions.CommunitySet,
				NextHopSet:   s.Conditions.NextHopSet,
			}
		}
		out = append(out, ps)
	}
	return out
}

func actionsToProto(a PolicyActions) *providerv1alpha1.PolicyActions {
	pa := &providerv1alpha1.PolicyActions{
		RouteDisposition: a.RouteDisposition,
		SetNextHop:       a.SetNextHop,
	}
	if a.SetCommunity != nil {
		pa.SetCommunity = &providerv1alpha1.SetCommunityAction{
			Communities: a.SetCommunity.Communities,
			Method:      a.SetCommunity.Method,
		}
	}
	if a.SetLocalPreference != nil {
		v := *a.SetLocalPreference
		pa.SetLocalPreference = &v
	}
	if a.SetMED != nil {
		v := *a.SetMED
		pa.SetMed = &v
	}
	return pa
}

func capsFromProto(cs *providerv1alpha1.CapabilitySet) CapabilitySet {
	if cs == nil {
		return CapabilitySet{}
	}
	afs := make([]AddressFamily, 0, len(cs.AddressFamilies))
	for _, af := range cs.AddressFamilies {
		afs = append(afs, AddressFamily{AFI: af.Afi, SAFI: af.Safi})
	}
	return CapabilitySet{
		AddressFamilies: afs,
		RouteReflection: cs.RouteReflection,
		BFD:             cs.Bfd,
	}
}
