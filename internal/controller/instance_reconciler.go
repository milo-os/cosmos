package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	"go.miloapis.com/cosmos/internal/provider"
)

// InstanceReconciler reconciles BGPInstance resources.
// It resolves the router ID, derives per-provider speaker configuration, and calls
// provider.ConfigureInstance for each matched BGPProvider.
type InstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Pool     *provider.Pool
	NodeName string // from NODE_NAME env var
}

// Reconcile handles BGPInstance events.
func (r *InstanceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var instance bgpv1alpha1.BGPInstance
	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &instance)
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&instance, Finalizer) {
		patch := client.MergeFrom(instance.DeepCopy())
		controllerutil.AddFinalizer(&instance, Finalizer)
		if err := r.Patch(ctx, &instance, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// List providers matching providerSelector.
	sel, err := metav1.LabelSelectorAsSelector(&instance.Spec.ProviderSelector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid providerSelector: %w", err)
	}

	var providerList providersv1alpha1.BGPProviderList
	if err := r.List(ctx, &providerList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list BGPProviders: %w", err)
	}

	if len(providerList.Items) == 0 {
		return r.setInstanceCondition(ctx, &instance, "ProviderNotMatched", metav1.ConditionTrue,
			"ProviderNotMatched", "no BGPProvider resources matched providerSelector")
	}

	var providerErrs []error
	for i := range providerList.Items {
		bp := &providerList.Items[i]
		if err := r.reconcileForProvider(ctx, &instance, bp); err != nil {
			providerErrs = append(providerErrs, fmt.Errorf("provider %s: %w", bp.Name, err))
		}
	}
	if len(providerErrs) > 0 {
		return ctrl.Result{}, errors.Join(providerErrs...)
	}
	return ctrl.Result{}, nil
}

// reconcileForProvider applies BGPInstance configuration to one provider.
func (r *InstanceReconciler) reconcileForProvider(
	ctx context.Context,
	instance *bgpv1alpha1.BGPInstance,
	bp *providersv1alpha1.BGPProvider,
) error {
	impl, ok := r.Pool.GetByName(bp.Name)
	if !ok {
		if r.NodeName != "" && bp.Labels[LabelNode] != r.NodeName {
			return nil
		}
		return r.writeInstanceProviderStatus(ctx, instance, bp.Name, bp.Spec.Type, false,
			"DaemonUnavailable", "provider not in pool — daemon may be starting")
	}

	// Derive router ID.
	routerID, err := r.resolveRouterID(ctx, instance, bp)
	if err != nil {
		return r.writeInstanceProviderStatus(ctx, instance, bp.Name, bp.Spec.Type, false,
			"RouterIDResolutionFailed", fmt.Sprintf("resolve router ID: %v", err))
	}

	// Determine listen port from spec. The API server applies the default (179)
	// on admission so this is non-nil for all current objects; the fallback
	// handles objects created before the default was introduced.
	var listenPort int32 = 179
	if instance.Spec.ListenPort != nil {
		listenPort = *instance.Spec.ListenPort
	}

	// Convert address families.
	families := make([]provider.AddressFamily, 0, len(instance.Spec.AddressFamilies))
	for _, af := range instance.Spec.AddressFamilies {
		families = append(families, provider.AddressFamily{AFI: af.AFI, SAFI: af.SAFI})
	}

	// Build timers with defaults.
	timers := provider.TimerConfig{HoldTime: 90, Keepalive: 30}
	if t := instance.Spec.Timers; t != nil {
		if t.DefaultHoldTime > 0 {
			timers.HoldTime = t.DefaultHoldTime
		}
		if t.DefaultKeepalive > 0 {
			timers.Keepalive = t.DefaultKeepalive
		}
	}

	// Build best path config.
	var bestPath provider.BestPathConfig
	if bp := instance.Spec.BestPath; bp != nil {
		bestPath.AlwaysCompareMed = bp.AlwaysCompareMed
		bestPath.DeterministicMed = bp.DeterministicMed
		bestPath.CompareRouterID = bp.CompareRouterID
	}

	// Build route reflector config.
	var rrConfig *provider.RouteReflectorConfig
	if rr := instance.Spec.RouteReflector; rr != nil {
		rrConfig = &provider.RouteReflectorConfig{ClusterID: rr.ClusterID}
	}

	spec := provider.InstanceSpec{
		ASNumber:       instance.Spec.ASNumber,
		RouterID:       routerID,
		ListenPort:     listenPort,
		Families:       families,
		Timers:         timers,
		BestPath:       bestPath,
		RouteReflector: rrConfig,
	}

	restarted, err := impl.ConfigureInstance(ctx, spec)
	if err != nil {
		return r.writeInstanceProviderStatus(ctx, instance, bp.Name, bp.Spec.Type, false,
			"ConfigurationFailed", fmt.Sprintf("ConfigureInstance: %v", err))
	}

	// When the remote agent was restarted, all peer state is wiped. Bump an
	// annotation on every BGPPeer that targets this provider so the
	// PeerReconciler re-applies their configuration.
	if restarted {
		r.invalidatePeersForProvider(ctx, bp.Name)
	}

	return r.writeInstanceProviderStatus(ctx, instance, bp.Name, bp.Spec.Type, true,
		"InstanceConfigured", fmt.Sprintf("speaker configured (AS=%d routerID=%s port=%d)", spec.ASNumber, spec.RouterID, spec.ListenPort))
}

// resolveRouterID computes the router ID for the given instance and provider.
func (r *InstanceReconciler) resolveRouterID(
	ctx context.Context,
	instance *bgpv1alpha1.BGPInstance,
	bp *providersv1alpha1.BGPProvider,
) (string, error) {
	if instance.Spec.RouterIDSource == "Manual" {
		return instance.Spec.RouterID, nil
	}

	// Auto: use the IPv6 InternalIP from the provider's node.
	nodeName := ""
	if bp.Annotations != nil {
		nodeName = bp.Annotations[LabelNode]
	}
	if nodeName == "" {
		// Fall back to the LabelNode label if annotation is absent.
		if bp.Labels != nil {
			nodeName = bp.Labels[LabelNode]
		}
	}
	if nodeName == "" {
		return "", fmt.Errorf("BGPProvider %s has no %s annotation or label", bp.Name, LabelNode)
	}

	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return "", fmt.Errorf("get node %s: %w", nodeName, err)
	}

	// Collect all IPv6 InternalIP addresses.
	var ipv6Addrs []string
	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeInternalIP {
			continue
		}
		ip := net.ParseIP(addr.Address)
		if ip == nil || ip.To4() != nil {
			continue // skip IPv4
		}
		ipv6Addrs = append(ipv6Addrs, addr.Address)
	}

	if len(ipv6Addrs) == 0 {
		return "", fmt.Errorf("node %s has no IPv6 InternalIP addresses", nodeName)
	}

	// Lexicographically first IPv6 address.
	sort.Strings(ipv6Addrs)
	selectedIP := net.ParseIP(ipv6Addrs[0])
	if selectedIP == nil {
		return "", fmt.Errorf("parse selected IPv6 address %q", ipv6Addrs[0])
	}

	return ipv6ToRouterID(selectedIP), nil
}

// writeInstanceProviderStatus writes per-provider status for a BGPInstance using server-side apply.
func (r *InstanceReconciler) writeInstanceProviderStatus(
	ctx context.Context,
	instance *bgpv1alpha1.BGPInstance,
	providerName, daemonType string,
	configured bool,
	reason, msg string,
) error {
	condStatus := metav1.ConditionFalse
	if configured {
		condStatus = metav1.ConditionTrue
	}
	cond := metav1.Condition{
		Type:               "InstanceConfigured",
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: instance.Generation,
	}

	// Find or create the provider status entry.
	updated := instance.DeepCopy()
	found := false
	for i, ps := range updated.Status.Providers {
		if ps.ProviderName == providerName {
			apimeta.SetStatusCondition(&updated.Status.Providers[i].Conditions, cond)
			found = true
			break
		}
	}
	if !found {
		updated.Status.Providers = append(updated.Status.Providers, bgpv1alpha1.ProviderStatus{
			ProviderName: providerName,
			Daemon:       daemonType,
		})
		apimeta.SetStatusCondition(&updated.Status.Providers[len(updated.Status.Providers)-1].Conditions, cond)
	}

	patch := client.MergeFrom(instance)
	patchErr := r.Status().Patch(ctx, updated, patch)
	if !configured {
		return errors.Join(fmt.Errorf("%s: %s", reason, msg), patchErr)
	}
	return patchErr
}

// invalidatePeersForProvider bumps a reconcile annotation on all BGPPeers that
// reference the given provider so the PeerReconciler re-applies their config.
// Called after a speaker restart that wipes all session state.
func (r *InstanceReconciler) invalidatePeersForProvider(ctx context.Context, providerName string) {
	var peerList bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &peerList); err != nil {
		log.Printf("bgp/instance: list BGPPeers for provider %s: %v", providerName, err)
		return
	}
	for i := range peerList.Items {
		peer := &peerList.Items[i]
		if peer.Spec.ProviderRef != providerName {
			continue
		}
		patch := client.MergeFrom(peer.DeepCopy())
		if peer.Annotations == nil {
			peer.Annotations = map[string]string{}
		}
		peer.Annotations["bgp.miloapis.com/speaker-restart"] = fmt.Sprintf("%d", peer.Generation)
		if err := r.Patch(ctx, peer, patch); err != nil {
			log.Printf("bgp/instance: touch BGPPeer %s: %v", peer.Name, err)
		}
	}
}

// setInstanceCondition writes a top-level status condition and returns an error
// so the controller re-queues with backoff until the underlying issue is resolved.
func (r *InstanceReconciler) setInstanceCondition(
	ctx context.Context,
	instance *bgpv1alpha1.BGPInstance,
	condType string,
	condStatus metav1.ConditionStatus,
	reason, msg string,
) (reconcile.Result, error) {
	updated := instance.DeepCopy()
	apimeta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: instance.Generation,
	})
	if err := r.Status().Patch(ctx, updated, client.MergeFrom(instance)); err != nil {
		log.Printf("bgp/instance: %s patch condition %s: %v", instance.Name, reason, err)
	}
	return ctrl.Result{}, fmt.Errorf("%s: %s", reason, msg)
}

// handleDelete removes provider configuration and the finalizer.
func (r *InstanceReconciler) handleDelete(ctx context.Context, instance *bgpv1alpha1.BGPInstance) error {
	if !controllerutil.ContainsFinalizer(instance, Finalizer) {
		return nil
	}

	// Check for referencing BGPPeer resources.
	var peerList bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &peerList); err != nil {
		return fmt.Errorf("list BGPPeers: %w", err)
	}
	for _, peer := range peerList.Items {
		if peer.Spec.InstanceRef == instance.Name {
			return fmt.Errorf("deletion blocked: BGPPeer %s references this instance", peer.Name)
		}
	}

	// Check for referencing BGPAdvertisement resources.
	var advList bgpv1alpha1.BGPAdvertisementList
	if err := r.List(ctx, &advList); err != nil {
		return fmt.Errorf("list BGPAdvertisements: %w", err)
	}
	for _, adv := range advList.Items {
		if adv.Spec.InstanceRef == instance.Name {
			return fmt.Errorf("deletion blocked: BGPAdvertisement %s references this instance", adv.Name)
		}
	}

	// Check for referencing BGPRoutePolicy resources.
	var policyList bgpv1alpha1.BGPRoutePolicyList
	if err := r.List(ctx, &policyList); err != nil {
		return fmt.Errorf("list BGPRoutePolicies: %w", err)
	}
	for _, pol := range policyList.Items {
		if pol.Spec.InstanceRef == instance.Name {
			return fmt.Errorf("deletion blocked: BGPRoutePolicy %s references this instance", pol.Name)
		}
	}

	// All clear — remove finalizer.
	patch := client.MergeFrom(instance.DeepCopy())
	controllerutil.RemoveFinalizer(instance, Finalizer)
	return r.Patch(ctx, instance, patch)
}

// mapProviderToInstances re-triggers reconciliation for all BGPInstances whose
// providerSelector matches a changed BGPProvider.
func (r *InstanceReconciler) mapProviderToInstances(ctx context.Context, obj client.Object) []reconcile.Request {
	bp, ok := obj.(*providersv1alpha1.BGPProvider)
	if !ok {
		return nil
	}
	var instanceList bgpv1alpha1.BGPInstanceList
	if err := r.List(ctx, &instanceList); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, instance := range instanceList.Items {
		sel, err := metav1.LabelSelectorAsSelector(&instance.Spec.ProviderSelector)
		if err == nil && sel.Matches(labels.Set(bp.Labels)) {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.Name}})
		}
	}
	return reqs
}

// SetupWithManager registers InstanceReconciler with controller-runtime.
func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPInstance{}).
		Watches(
			&providersv1alpha1.BGPProvider{},
			handler.EnqueueRequestsFromMapFunc(r.mapProviderToInstances),
		).
		Complete(r)
}
