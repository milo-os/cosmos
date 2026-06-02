package controller

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bgpv1alpha1 "go.miloapis.com/bgp/api/v1alpha1"
	providersv1alpha1 "go.miloapis.com/bgp/api/providers/v1alpha1"
	"go.miloapis.com/bgp/internal/provider"
	frrprovider "go.miloapis.com/bgp/internal/provider/frr"
	gobgpprovider "go.miloapis.com/bgp/internal/provider/gobgp"
)

const (
	// LabelManagedBy is the label key that records which controller manages a resource.
	LabelManagedBy = "bgp.miloapis.com/managed-by"
	// LabelSessionName records the BGPSession that generated a BGPPeer.
	LabelSessionName = "bgp.miloapis.com/session-name"
	// AnnotationSessionUID records the UID of the BGPSession that generated a BGPPeer.
	AnnotationSessionUID = "bgp.miloapis.com/session-uid"
	// LabelDaemon records which BGP daemon type backs a provider.
	LabelDaemon = "bgp.miloapis.com/daemon"
	// LabelNode records the Kubernetes node name a provider runs on.
	LabelNode = "bgp.miloapis.com/node"
	// LabelManagedByBootstrap indicates the resource was created at controller bootstrap.
	LabelManagedByBootstrap = "cosmos-bootstrap"
	// LabelManagedByManagement indicates the resource was created by the management cluster controller.
	LabelManagedByManagement = "cosmos-management"

	// Finalizer is the finalizer added to resources managed by this controller.
	Finalizer = "cosmos.bgp.miloapis.com/cleanup"

	// providerHealthRequeue is the interval at which provider health is rechecked.
	providerHealthRequeue = 30 * time.Second
)

// ProviderReconciler reconciles BGPProvider resources.
// It auto-bootstraps local providers at startup and maintains provider health status.
//
// Active in: pop, infra.
type ProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *provider.Registry
	NodeName string // from NODE_NAME env var
}

// Reconcile handles BGPProvider events.
func (r *ProviderReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var bgpProvider providersv1alpha1.BGPProvider
	if err := r.Get(ctx, req.NamespacedName, &bgpProvider); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !bgpProvider.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &bgpProvider)
	}

	// Only manage providers that belong to this node.
	// Each DaemonSet pod's controller only connects to its own local daemon (localhost).
	// Attempting to reconcile remote-node providers would register them against the
	// wrong daemon and corrupt peer state on unrelated GoBGP/FRR instances.
	if r.NodeName != "" && bgpProvider.Labels[LabelNode] != r.NodeName {
		return ctrl.Result{}, nil
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&bgpProvider, Finalizer) {
		patch := client.MergeFrom(bgpProvider.DeepCopy())
		controllerutil.AddFinalizer(&bgpProvider, Finalizer)
		if err := r.Patch(ctx, &bgpProvider, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &bgpProvider); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Resolve endpoint from spec.
	endpoint, err := endpointFromSpec(&bgpProvider)
	if err != nil {
		return r.setProviderCondition(ctx, &bgpProvider, "Ready", metav1.ConditionFalse, "InvalidEndpoint",
			fmt.Sprintf("spec has no endpoint configured: %v", err))
	}

	// v1alpha1 restriction: only loopback endpoints allowed.
	if !isLoopback(endpoint) {
		return r.setProviderCondition(ctx, &bgpProvider, "Ready", metav1.ConditionFalse, "RemoteProviderNotSupported",
			fmt.Sprintf("endpoint %q is not a loopback address; remote providers are not supported in v1alpha1", endpoint))
	}

	// Validate endpoint is well-formed (host:port).
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return r.setProviderCondition(ctx, &bgpProvider, "Ready", metav1.ConditionFalse, "InvalidEndpoint",
			fmt.Sprintf("endpoint %q is malformed: %v", endpoint, err))
	}

	// Ensure provider implementation exists in the registry.
	if _, ok := r.Registry.Get(bgpProvider.Name); !ok {
		impl, err := r.newProviderImpl(&bgpProvider, endpoint)
		if err != nil {
			return r.setProviderCondition(ctx, &bgpProvider, "Ready", metav1.ConditionFalse, "DaemonUnreachable",
				fmt.Sprintf("create provider implementation: %v", err))
		}
		r.Registry.Set(bgpProvider.Name, impl)
	}

	impl, _ := r.Registry.Get(bgpProvider.Name)

	// Health probe.
	readyErr := impl.Ready(ctx)
	if readyErr != nil {
		log.Printf("bgp/provider: %s daemon not responsive: %v", bgpProvider.Name, readyErr)
		// Remove stale impl so it gets recreated with a fresh connection next cycle.
		r.Registry.Delete(bgpProvider.Name)
		return r.setProviderCondition(ctx, &bgpProvider, "Ready", metav1.ConditionFalse, "DaemonUnreachable",
			fmt.Sprintf("daemon not responsive: %v", readyErr))
	}

	// Query capabilities.
	caps, capsErr := impl.Capabilities(ctx)
	if capsErr != nil {
		log.Printf("bgp/provider: %s capabilities query failed: %v", bgpProvider.Name, capsErr)
	}

	// Update status.
	patch := client.MergeFrom(bgpProvider.DeepCopy())
	apimeta.SetStatusCondition(&bgpProvider.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "DaemonResponsive",
		Message:            fmt.Sprintf("daemon at %s is responsive", endpoint),
		ObservedGeneration: bgpProvider.Generation,
	})
	bgpProvider.Status.ResolvedEndpoint = endpoint
	bgpProvider.Status.Daemon = bgpProvider.Spec.Type
	if capsErr == nil {
		bgpProvider.Status.Capabilities = capabilitiesToStatus(caps)
	}
	if err := r.Status().Patch(ctx, &bgpProvider, patch); err != nil {
		log.Printf("bgp/provider: patch status: %v", err)
	}

	// Requeue for periodic health check.
	return ctrl.Result{RequeueAfter: providerHealthRequeue}, nil
}

// handleDelete blocks deletion if any referencing resources exist, then removes the finalizer.
func (r *ProviderReconciler) handleDelete(ctx context.Context, bgpProvider *providersv1alpha1.BGPProvider) error {
	if !controllerutil.ContainsFinalizer(bgpProvider, Finalizer) {
		return nil
	}

	// Check for referencing BGPInstance resources.
	var instanceList bgpv1alpha1.BGPInstanceList
	if err := r.List(ctx, &instanceList); err != nil {
		return fmt.Errorf("list BGPInstances: %w", err)
	}
	for _, inst := range instanceList.Items {
		sel, err := metav1.LabelSelectorAsSelector(&inst.Spec.ProviderSelector)
		if err != nil {
			continue
		}
		if sel.Matches(labelsForProvider(bgpProvider)) {
			return r.blockDeletion(ctx, bgpProvider, "ReferencedByInstance",
				fmt.Sprintf("BGPInstance %s references this provider", inst.Name))
		}
	}

	// Check for referencing BGPPeer resources.
	var peerList bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &peerList); err != nil {
		return fmt.Errorf("list BGPPeers: %w", err)
	}
	for _, peer := range peerList.Items {
		if peer.Spec.ProviderRef == bgpProvider.Name {
			return r.blockDeletion(ctx, bgpProvider, "ReferencedByPeer",
				fmt.Sprintf("BGPPeer %s references this provider", peer.Name))
		}
		if peer.Spec.ProviderSelector != nil {
			sel, err := metav1.LabelSelectorAsSelector(peer.Spec.ProviderSelector)
			if err == nil && sel.Matches(labelsForProvider(bgpProvider)) {
				return r.blockDeletion(ctx, bgpProvider, "ReferencedByPeer",
					fmt.Sprintf("BGPPeer %s selector matches this provider", peer.Name))
			}
		}
	}

	// Clear from registry, then remove finalizer.
	r.Registry.Delete(bgpProvider.Name)
	patch := client.MergeFrom(bgpProvider.DeepCopy())
	controllerutil.RemoveFinalizer(bgpProvider, Finalizer)
	return r.Patch(ctx, bgpProvider, patch)
}

// blockDeletion sets a DeletionBlocked condition and returns an error to prevent deletion.
func (r *ProviderReconciler) blockDeletion(ctx context.Context, bgpProvider *providersv1alpha1.BGPProvider, reason, msg string) error {
	patch := client.MergeFrom(bgpProvider.DeepCopy())
	apimeta.SetStatusCondition(&bgpProvider.Status.Conditions, metav1.Condition{
		Type:               "DeletionBlocked",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: bgpProvider.Generation,
	})
	_ = r.Status().Patch(ctx, bgpProvider, patch)
	return fmt.Errorf("deletion blocked: %s", msg)
}

// setProviderCondition updates one condition and returns the appropriate result.
func (r *ProviderReconciler) setProviderCondition(
	ctx context.Context,
	bgpProvider *providersv1alpha1.BGPProvider,
	condType string,
	status metav1.ConditionStatus,
	reason, msg string,
) (reconcile.Result, error) {
	patch := client.MergeFrom(bgpProvider.DeepCopy())
	apimeta.SetStatusCondition(&bgpProvider.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: bgpProvider.Generation,
	})
	if err := r.Status().Patch(ctx, bgpProvider, patch); err != nil {
		log.Printf("bgp/provider: patch status: %v", err)
	}
	// Hard validation errors do not requeue — the spec must be fixed.
	if reason == "InvalidEndpoint" || reason == "RemoteProviderNotSupported" {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: providerHealthRequeue}, nil
}

// newProviderImpl creates the correct in-process provider implementation.
func (r *ProviderReconciler) newProviderImpl(bgpProvider *providersv1alpha1.BGPProvider, endpoint string) (provider.Provider, error) {
	switch bgpProvider.Spec.Type {
	case "FRR":
		return frrprovider.New(endpoint), nil
	case "GoBGP":
		return gobgpprovider.New(endpoint)
	default:
		return nil, fmt.Errorf("unknown provider type %q", bgpProvider.Spec.Type)
	}
}

// bootstrapLocalProviders creates BGPProvider resources for FRR and GoBGP on the
// local node if they do not exist. Called once at controller startup by the Manager.
func (r *ProviderReconciler) bootstrapLocalProviders(ctx context.Context) error {
	if r.NodeName == "" {
		log.Printf("bgp/provider: NODE_NAME not set — skipping bootstrap")
		return nil
	}

	// Fetch the Kubernetes Node to copy its labels.
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: r.NodeName}, &node); err != nil {
		return fmt.Errorf("get node %s: %w", r.NodeName, err)
	}

	daemons := []struct {
		suffix   string
		specType string
		endpoint string
	}{
		{"frr", "FRR", "localhost:50051"},
		{"gobgp", "GoBGP", "localhost:50051"},
	}

	for _, d := range daemons {
		name := r.NodeName + "-" + d.suffix
		var existing providersv1alpha1.BGPProvider
		if err := r.Get(ctx, types.NamespacedName{Name: name}, &existing); err == nil {
			continue // already exists
		}

		labels := make(map[string]string)
		for k, v := range node.Labels {
			labels[k] = v
		}
		labels[LabelManagedBy] = LabelManagedByBootstrap
		labels[LabelDaemon] = d.specType
		labels[LabelNode] = r.NodeName

		bp := &providersv1alpha1.BGPProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
				Annotations: map[string]string{
					LabelNode: r.NodeName,
				},
			},
			Spec: buildProviderSpec(d.specType, d.endpoint),
		}

		if err := r.Create(ctx, bp); err != nil {
			log.Printf("bgp/provider: bootstrap %s: %v", name, err)
		} else {
			log.Printf("bgp/provider: bootstrapped %s (type=%s endpoint=%s)", name, d.specType, d.endpoint)
		}
	}
	return nil
}

// buildProviderSpec constructs a BGPProviderSpec for the given daemon type.
func buildProviderSpec(daemonType, endpoint string) providersv1alpha1.BGPProviderSpec {
	spec := providersv1alpha1.BGPProviderSpec{Type: daemonType}
	switch daemonType {
	case "FRR":
		spec.FRR = &providersv1alpha1.FRRProviderConfig{Endpoint: endpoint}
	case "GoBGP":
		spec.GoBGP = &providersv1alpha1.GoBGPProviderConfig{Endpoint: endpoint}
	}
	return spec
}

// endpointFromSpec extracts the configured endpoint from a BGPProvider spec.
func endpointFromSpec(bgpProvider *providersv1alpha1.BGPProvider) (string, error) {
	switch bgpProvider.Spec.Type {
	case "FRR":
		if bgpProvider.Spec.FRR != nil {
			return bgpProvider.Spec.FRR.Endpoint, nil
		}
	case "GoBGP":
		if bgpProvider.Spec.GoBGP != nil {
			return bgpProvider.Spec.GoBGP.Endpoint, nil
		}
	}
	return "", fmt.Errorf("no endpoint configured for type %q", bgpProvider.Spec.Type)
}

// isLoopback returns true when endpoint is a loopback address or "localhost".
func isLoopback(endpoint string) bool {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		// Try treating the whole string as a host.
		host = endpoint
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost"
	}
	return ip.IsLoopback()
}

// labelsForProvider returns the labels of a BGPProvider as a labels.Set for selector matching.
func labelsForProvider(bgpProvider *providersv1alpha1.BGPProvider) labels.Set {
	if bgpProvider.Labels == nil {
		return labels.Set{}
	}
	return labels.Set(bgpProvider.Labels)
}

// capabilitiesToStatus converts a provider.CapabilitySet to the providers API status type.
func capabilitiesToStatus(caps provider.CapabilitySet) *providersv1alpha1.ProviderCapabilities {
	afs := make([]providersv1alpha1.AddressFamilyCapability, 0, len(caps.AddressFamilies))
	for _, af := range caps.AddressFamilies {
		afs = append(afs, providersv1alpha1.AddressFamilyCapability{AFI: af.AFI, SAFI: af.SAFI})
	}
	return &providersv1alpha1.ProviderCapabilities{
		AddressFamilies: afs,
		RouteReflection: caps.RouteReflection,
		BFD:             caps.BFD,
	}
}

// ipv6ToRouterID maps the last 4 bytes of an IPv6 address to a dotted-quad IPv4 string.
// Used for automatic router ID derivation on IPv6-only nodes.
func ipv6ToRouterID(ip net.IP) string {
	ip16 := ip.To16()
	if ip16 == nil {
		return "0.0.0.0"
	}
	last4 := ip16[12:]
	n := binary.BigEndian.Uint32(last4)
	return fmt.Sprintf("%d.%d.%d.%d", byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// SetupWithManager registers ProviderReconciler with controller-runtime.
func (r *ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&providersv1alpha1.BGPProvider{}).
		Complete(r)
}
