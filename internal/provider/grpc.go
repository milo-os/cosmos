package provider

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	providerv1alpha1 "go.miloapis.com/cosmos/api/proto/bgp/provider/v1alpha1"
)

// GRPCProvider implements Provider by delegating each call to a remote
// BGPProviderService over gRPC. It holds no mutable state.
type GRPCProvider struct {
	client providerv1alpha1.BGPProviderServiceClient
}

// NewGRPCProvider wraps conn in a GRPCProvider.
func NewGRPCProvider(conn grpc.ClientConnInterface) *GRPCProvider {
	return &GRPCProvider{client: providerv1alpha1.NewBGPProviderServiceClient(conn)}
}

func (g *GRPCProvider) ConfigureInstance(ctx context.Context, spec InstanceSpec) (bool, error) {
	resp, err := g.client.ConfigureSpeaker(ctx, instanceSpecToProto(spec))
	if err != nil {
		return false, grpcErr(err)
	}
	return resp.Restarted, nil
}

func (g *GRPCProvider) AddOrUpdatePeer(ctx context.Context, peer PeerSpec) error {
	_, err := g.client.AddOrUpdatePeer(ctx, &providerv1alpha1.AddOrUpdatePeerRequest{Peer: peerSpecToProto(peer)})
	return grpcErr(err)
}

func (g *GRPCProvider) DeletePeer(ctx context.Context, address string) error {
	_, err := g.client.DeletePeer(ctx, &providerv1alpha1.DeletePeerRequest{Address: address})
	return grpcErr(err)
}

func (g *GRPCProvider) AddOrUpdateAdvertisement(ctx context.Context, adv AdvertisementSpec) error {
	_, err := g.client.AddOrUpdateAdvertisement(ctx, &providerv1alpha1.AddOrUpdateAdvertisementRequest{
		Advertisement: &providerv1alpha1.AdvertisementSpec{
			Prefixes:      adv.Prefixes,
			PeerAddresses: adv.PeerAddresses,
		},
	})
	return grpcErr(err)
}

func (g *GRPCProvider) DeleteAdvertisement(ctx context.Context, prefix string) error {
	_, err := g.client.DeleteAdvertisement(ctx, &providerv1alpha1.DeleteAdvertisementRequest{Prefix: prefix})
	return grpcErr(err)
}

func (g *GRPCProvider) AddOrUpdatePolicy(ctx context.Context, pol PolicySpec) error {
	_, err := g.client.AddOrUpdatePolicy(ctx, &providerv1alpha1.AddOrUpdatePolicyRequest{Policy: policySpecToProto(pol)})
	return grpcErr(err)
}

func (g *GRPCProvider) DeletePolicy(ctx context.Context, policyName string) error {
	_, err := g.client.DeletePolicy(ctx, &providerv1alpha1.DeletePolicyRequest{PolicyName: policyName})
	return grpcErr(err)
}

func (g *GRPCProvider) Ready(ctx context.Context) error {
	_, err := g.client.Ready(ctx, &providerv1alpha1.ReadyRequest{})
	return grpcErr(err)
}

func (g *GRPCProvider) Capabilities(ctx context.Context) (CapabilitySet, error) {
	resp, err := g.client.Capabilities(ctx, &providerv1alpha1.CapabilitiesRequest{})
	if err != nil {
		return CapabilitySet{}, grpcErr(err)
	}
	return capsFromProto(resp.Capabilities), nil
}

// grpcErr passes through Unavailable and DeadlineExceeded unwrapped.
// All other non-OK codes are wrapped.
func grpcErr(err error) error {
	if err == nil {
		return nil
	}
	code := status.Code(err)
	if code == codes.Unavailable || code == codes.DeadlineExceeded {
		return err
	}
	return fmt.Errorf("grpc %s: %w", code, err)
}
