package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"

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

	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	"go.miloapis.com/cosmos/internal/provider"
)

// PeerReconciler reconciles BGPPeer resources.
// It resolves the BGPInstance, matches providers, and calls provider.AddOrUpdatePeer.
//
// Active in: pop, infra.
type PeerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *provider.Registry
	NodeName string // from NODE_NAME env var; used to distinguish local vs remote providers
}

// Reconcile handles BGPPeer events.
func (r *PeerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var peer bgpv1alpha1.BGPPeer
	if err := r.Get(ctx, req.NamespacedName, &peer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !peer.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &peer)
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&peer, Finalizer) {
		patch := client.MergeFrom(peer.DeepCopy())
		controllerutil.AddFinalizer(&peer, Finalizer)
		if err := r.Patch(ctx, &peer, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &peer); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Resolve BGPInstance.
	var instance bgpv1alpha1.BGPInstance
	if err := r.Get(ctx, types.NamespacedName{Name: peer.Spec.InstanceRef}, &instance); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return r.setPeerCondition(ctx, &peer, "InstanceNotFound", metav1.ConditionTrue,
			"InstanceNotFound", fmt.Sprintf("BGPInstance %q not found", peer.Spec.InstanceRef))
	}

	// Collect matched providers.
	providers, err := r.matchedProviders(ctx, &peer)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(providers) == 0 {
		return r.setPeerCondition(ctx, &peer, "ProviderNotMatched", metav1.ConditionTrue,
			"ProviderNotMatched", "no BGPProvider resources matched")
	}

	for _, bp := range providers {
		if err := r.reconcileForProvider(ctx, &peer, &instance, bp); err != nil {
			return ctrl.Result{}, fmt.Errorf("provider %s: %w", bp.Name, err)
		}
	}
	return ctrl.Result{}, nil
}

// matchedProviders returns all BGPProvider resources this peer should be reconciled against.
func (r *PeerReconciler) matchedProviders(ctx context.Context, peer *bgpv1alpha1.BGPPeer) ([]providersv1alpha1.BGPProvider, error) {
	if peer.Spec.ProviderRef != "" {
		var bp providersv1alpha1.BGPProvider
		if err := r.Get(ctx, types.NamespacedName{Name: peer.Spec.ProviderRef}, &bp); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return nil, err
			}
			return nil, nil
		}
		return []providersv1alpha1.BGPProvider{bp}, nil
	}

	if peer.Spec.ProviderSelector != nil {
		sel, err := metav1.LabelSelectorAsSelector(peer.Spec.ProviderSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid providerSelector: %w", err)
		}
		var list providersv1alpha1.BGPProviderList
		if err := r.List(ctx, &list, &client.ListOptions{LabelSelector: sel}); err != nil {
			return nil, fmt.Errorf("list BGPProviders: %w", err)
		}
		return list.Items, nil
	}

	return nil, nil
}

// reconcileForProvider configures one peer on one provider.
func (r *PeerReconciler) reconcileForProvider(
	ctx context.Context,
	peer *bgpv1alpha1.BGPPeer,
	instance *bgpv1alpha1.BGPInstance,
	bp providersv1alpha1.BGPProvider,
) error {
	// Verify that the BGPInstance's providerSelector matches this provider.
	instSel, err := metav1.LabelSelectorAsSelector(&instance.Spec.ProviderSelector)
	if err == nil && !instSel.Matches(labels.Set(bp.Labels)) {
		return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, false,
			"InstanceNotMatchedOnProvider",
			fmt.Sprintf("BGPInstance %s providerSelector does not match provider %s", instance.Name, bp.Name))
	}

	// Self-peer check: skip if the peer address is this node's own IPv6 InternalIP.
	if r.isSelfPeer(ctx, peer.Spec.Address, &bp) {
		return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, false,
			"SkippedSelfPeer",
			fmt.Sprintf("peer address %s is this provider's node IPv6 InternalIP", peer.Spec.Address))
	}

	// Resolve timers: peer overrides instance defaults.
	holdTime := int32(90)
	keepalive := int32(30)
	if t := instance.Spec.Timers; t != nil {
		if t.DefaultHoldTime > 0 {
			holdTime = t.DefaultHoldTime
		}
		if t.DefaultKeepalive > 0 {
			keepalive = t.DefaultKeepalive
		}
	}
	if t := peer.Spec.Timers; t != nil {
		if t.HoldTime != nil {
			holdTime = *t.HoldTime
		}
		if t.Keepalive != nil {
			keepalive = *t.Keepalive
		}
	}

	// Determine session type.
	isIBGP := peer.Spec.ASNumber == instance.Spec.ASNumber

	// iBGP + eBGP multihop is invalid.
	if isIBGP && peer.Spec.EBGPMultihop != nil {
		return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, false,
			"InvalidForIBGP", "ebgpMultihop is invalid for iBGP sessions (same ASNumber as instance)")
	}

	// Resolve address families.
	afs := instance.Spec.AddressFamilies
	if len(peer.Spec.AddressFamilies) > 0 {
		afs = peer.Spec.AddressFamilies
	}
	families := make([]provider.AddressFamily, 0, len(afs))
	for _, af := range afs {
		families = append(families, provider.AddressFamily{AFI: af.AFI, SAFI: af.SAFI})
	}

	// Resolve password from Secret if configured.
	password := ""
	if ref := peer.Spec.PasswordSecretRef; ref != nil {
		secret, err := r.resolveSecret(ctx, ref.Name, ref.Key)
		if err != nil {
			return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, false,
				"SecretNotFound", fmt.Sprintf("resolve password secret: %v", err))
		}
		password = secret
	}

	var allowAsIn int32
	if peer.Spec.AllowAsIn != nil {
		allowAsIn = *peer.Spec.AllowAsIn
	}

	sessionType := "eBGP"
	if isIBGP {
		sessionType = "iBGP"
	}

	peerSpec := provider.PeerSpec{
		Address:              peer.Spec.Address,
		ASNumber:             peer.Spec.ASNumber,
		Families:             families,
		Timers:               provider.TimerConfig{HoldTime: holdTime, Keepalive: keepalive},
		AllowAsIn:            allowAsIn,
		RouteReflectorClient: peer.Spec.RouteReflectorClient,
		Passive:              peer.Spec.Passive,
		EBGPMultihop:         peer.Spec.EBGPMultihop,
		TTLSecurity:          peer.Spec.TTLSecurity,
		Password:             password,
	}

	impl, ok := r.Registry.Get(bp.Name)
	if !ok {
		// Remote providers (belonging to a different node) are never in this controller's
		// registry. Skip them silently — they are reconciled by the correct node's controller.
		if r.NodeName != "" && bp.Labels[LabelNode] != r.NodeName {
			return nil
		}
		// Local provider not yet in registry — daemon may still be starting up.
		return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, false,
			"DaemonUnavailable", "provider not in registry — daemon may be starting")
	}

	if err := impl.AddOrUpdatePeer(ctx, peerSpec); err != nil {
		return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, false,
			"PeerConfigurationFailed", fmt.Sprintf("AddOrUpdatePeer: %v", err))
	}

	return r.writePeerProviderStatus(ctx, peer, bp.Name, bp.Spec.Type, true,
		"PeerConfigured",
		fmt.Sprintf("peer %s configured (%s, AS=%d)", peer.Spec.Address, sessionType, peer.Spec.ASNumber))
}

// isSelfPeer returns true if the given address is this provider's node IPv6 InternalIP.
func (r *PeerReconciler) isSelfPeer(ctx context.Context, address string, bp *providersv1alpha1.BGPProvider) bool {
	nodeName := ""
	if bp.Annotations != nil {
		nodeName = bp.Annotations[LabelNode]
	}
	if nodeName == "" && bp.Labels != nil {
		nodeName = bp.Labels[LabelNode]
	}
	if nodeName == "" {
		return false
	}

	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return false
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeInternalIP {
			continue
		}
		ip := net.ParseIP(addr.Address)
		if ip == nil || ip.To4() != nil {
			continue // skip IPv4
		}
		if addr.Address == address {
			return true
		}
	}
	return false
}

// resolveSecret fetches a key from a Kubernetes Secret.
func (r *PeerReconciler) resolveSecret(ctx context.Context, secretName, key string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secretName}, &secret); err != nil {
		return "", fmt.Errorf("get secret %s: %w", secretName, err)
	}
	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s", key, secretName)
	}
	return string(val), nil
}

// writePeerProviderStatus writes per-provider status for a BGPPeer.
func (r *PeerReconciler) writePeerProviderStatus(
	ctx context.Context,
	peer *bgpv1alpha1.BGPPeer,
	providerName, daemonType string,
	configured bool,
	reason, msg string,
) error {
	condStatus := metav1.ConditionFalse
	if configured {
		condStatus = metav1.ConditionTrue
	}
	cond := metav1.Condition{
		Type:               "PeerConfigured",
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: peer.Generation,
	}

	updated := peer.DeepCopy()
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

	patch := client.MergeFrom(peer)
	patchErr := r.Status().Patch(ctx, updated, patch)
	if !configured {
		return errors.Join(fmt.Errorf("%s: %s", reason, msg), patchErr)
	}
	return patchErr
}

// setPeerCondition writes a top-level status condition and returns an error so
// the controller re-queues with backoff until the underlying issue is resolved.
func (r *PeerReconciler) setPeerCondition(
	ctx context.Context,
	peer *bgpv1alpha1.BGPPeer,
	condType string,
	condStatus metav1.ConditionStatus,
	reason, msg string,
) (reconcile.Result, error) {
	updated := peer.DeepCopy()
	apimeta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: peer.Generation,
	})
	if err := r.Status().Patch(ctx, updated, client.MergeFrom(peer)); err != nil {
		log.Printf("bgp/peer: %s patch condition %s: %v", peer.Name, reason, err)
	}
	return ctrl.Result{}, fmt.Errorf("%s: %s", reason, msg)
}

// handleDelete calls DeletePeer on all matching providers before removing the finalizer.
func (r *PeerReconciler) handleDelete(ctx context.Context, peer *bgpv1alpha1.BGPPeer) error {
	if !controllerutil.ContainsFinalizer(peer, Finalizer) {
		return nil
	}

	providers, err := r.matchedProviders(ctx, peer)
	if err != nil {
		return fmt.Errorf("list providers for deletion: %w", err)
	}

	// Snapshot all BGPPeers once for the duplicate-owner check below.
	var allPeers bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &allPeers); err != nil {
		return fmt.Errorf("list BGPPeers for deletion check: %w", err)
	}

	blocked := false
	for _, bp := range providers {
		// Skip DeletePeer if another live BGPPeer still owns this (provider, address) pair.
		// GoBGP holds one session per address; removing it here would break the other peer.
		otherExists := false
		for _, other := range allPeers.Items {
			if other.Name == peer.Name || !other.DeletionTimestamp.IsZero() {
				continue
			}
			if other.Spec.ProviderRef == bp.Name && other.Spec.Address == peer.Spec.Address {
				otherExists = true
				break
			}
		}
		if otherExists {
			log.Printf("bgp/peer: delete %s: another BGPPeer owns %s on %s — skipping DeletePeer", peer.Name, peer.Spec.Address, bp.Name)
			continue
		}

		impl, ok := r.Registry.Get(bp.Name)
		if !ok {
			log.Printf("bgp/peer: delete %s: provider %s not in registry — skipping", peer.Name, bp.Name)
			continue
		}
		if err := impl.DeletePeer(ctx, peer.Spec.Address); err != nil {
			log.Printf("bgp/peer: delete peer %s on provider %s: %v", peer.Spec.Address, bp.Name, err)
			blocked = true
		}
	}

	if blocked {
		return fmt.Errorf("deletion blocked: daemon unavailable for one or more providers")
	}

	patch := client.MergeFrom(peer.DeepCopy())
	controllerutil.RemoveFinalizer(peer, Finalizer)
	return r.Patch(ctx, peer, patch)
}

// mapInstanceToPeers re-triggers reconciliation for all BGPPeers that reference
// a changed BGPInstance, so peers react when their instance appears or changes.
func (r *PeerReconciler) mapInstanceToPeers(ctx context.Context, obj client.Object) []reconcile.Request {
	instance, ok := obj.(*bgpv1alpha1.BGPInstance)
	if !ok {
		return nil
	}
	var peerList bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &peerList); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, peer := range peerList.Items {
		if peer.Spec.InstanceRef == instance.Name {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: peer.Name}})
		}
	}
	return reqs
}

// mapProviderToPeers re-triggers reconciliation for all BGPPeers that target a
// changed BGPProvider, bypassing exponential backoff so peers configure
// immediately when a daemon becomes available after a DaemonUnavailable failure.
func (r *PeerReconciler) mapProviderToPeers(ctx context.Context, obj client.Object) []reconcile.Request {
	bp, ok := obj.(*providersv1alpha1.BGPProvider)
	if !ok {
		return nil
	}
	var peerList bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &peerList); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, peer := range peerList.Items {
		if peer.Spec.ProviderRef == bp.Name {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: peer.Name}})
			continue
		}
		if peer.Spec.ProviderSelector != nil {
			sel, err := metav1.LabelSelectorAsSelector(peer.Spec.ProviderSelector)
			if err == nil && sel.Matches(labels.Set(bp.Labels)) {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: peer.Name}})
			}
		}
	}
	return reqs
}

// SetupWithManager registers PeerReconciler with controller-runtime.
func (r *PeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPPeer{}).
		Watches(
			&bgpv1alpha1.BGPInstance{},
			handler.EnqueueRequestsFromMapFunc(r.mapInstanceToPeers),
		).
		Watches(
			&providersv1alpha1.BGPProvider{},
			handler.EnqueueRequestsFromMapFunc(r.mapProviderToPeers),
		).
		Complete(r)
}
