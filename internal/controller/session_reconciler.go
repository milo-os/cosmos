package controller

import (
	"context"
	"fmt"
	"log"
	"net"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
)

// SessionReconciler reconciles BGPSession resources.
//
// In pop/infra clusters: generates BGPPeer resources from BGPSession spec.
// In management clusters: validates BGPSession resources and writes status.
// TODO: implement management-side session write logic (Karmada propagation).
//
// Active in: pop, infra, management.
type SessionReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ClusterRole string
}

// Reconcile handles BGPSession events.
func (r *SessionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var session bgpv1alpha1.BGPSession
	if err := r.Get(ctx, req.NamespacedName, &session); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !session.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &session)
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&session, Finalizer) {
		patch := client.MergeFrom(session.DeepCopy())
		controllerutil.AddFinalizer(&session, Finalizer)
		if err := r.Patch(ctx, &session, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &session); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Route to the correct path based on cluster role.
	switch r.ClusterRole {
	case "management":
		// TODO: implement management-side session validation and status update.
		// Management cluster cosmos writes BGPSession; Karmada handles propagation.
		return r.reconcileManagement(ctx, &session)
	default:
		// pop, infra
		return r.reconcilePopInfra(ctx, &session)
	}
}

// reconcileManagement handles the management-cluster path.
// TODO: full management-side session reconciliation (write BGPSession from higher-level objects).
func (r *SessionReconciler) reconcileManagement(ctx context.Context, session *bgpv1alpha1.BGPSession) (reconcile.Result, error) {
	patch := client.MergeFrom(session.DeepCopy())
	apimeta.SetStatusCondition(&session.Status.Conditions, metav1.Condition{
		Type:               "PeersReconciled",
		Status:             metav1.ConditionTrue,
		Reason:             "ManagementCluster",
		Message:            "management cluster: BGPSession validated",
		ObservedGeneration: session.Generation,
	})
	if err := r.Status().Patch(ctx, session, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch status: %w", err)
	}
	return ctrl.Result{}, nil
}

// reconcilePopInfra handles the pop/infra cluster path.
// It generates BGPPeer resources from the BGPSession spec.
func (r *SessionReconciler) reconcilePopInfra(ctx context.Context, session *bgpv1alpha1.BGPSession) (reconcile.Result, error) {
	// Sessions from external peer refs have no BGPPeer to generate.
	if session.Spec.FromExternalPeerRef != nil {
		return r.reconcileExternalRef(ctx, session)
	}

	if session.Spec.FromProviderSelector == nil {
		return ctrl.Result{}, fmt.Errorf("session %s has neither fromProviderSelector nor fromExternalPeerRef", session.Name)
	}

	// List local BGPProvider resources matching fromProviderSelector.
	sel, err := metav1.LabelSelectorAsSelector(session.Spec.FromProviderSelector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid fromProviderSelector: %w", err)
	}

	var providerList providersv1alpha1.BGPProviderList
	if err := r.List(ctx, &providerList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list BGPProviders: %w", err)
	}

	if len(providerList.Items) == 0 {
		patch := client.MergeFrom(session.DeepCopy())
		apimeta.SetStatusCondition(&session.Status.Conditions, metav1.Condition{
			Type:               "PeersReconciled",
			Status:             metav1.ConditionFalse,
			Reason:             "NoMatchedProviders",
			Message:            "fromProviderSelector matched no BGPProvider resources",
			ObservedGeneration: session.Generation,
		})
		if err := r.Status().Patch(ctx, session, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Build the expected set of BGPPeer resources.
	expectedPeers, err := r.buildExpectedPeers(ctx, session, providerList.Items)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("build expected peers: %w", err)
	}

	// List all existing BGPPeer resources labelled by this session.
	var existingPeers bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &existingPeers, client.MatchingLabels{
		LabelSessionName: session.Name,
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list existing BGPPeers: %w", err)
	}

	// Diff and apply.
	created, updated, deleted, err := r.syncPeers(ctx, expectedPeers, existingPeers.Items)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("sync peers: %w", err)
	}

	if created+updated+deleted > 0 {
		log.Printf("bgp/session: %s peers: +%d ~%d -%d", session.Name, created, updated, deleted)
	}

	// Update BGPSession status.
	patch := client.MergeFrom(session.DeepCopy())
	apimeta.SetStatusCondition(&session.Status.Conditions, metav1.Condition{
		Type:               "PeersReconciled",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("%d peer(s) reconciled", len(expectedPeers)),
		ObservedGeneration: session.Generation,
	})
	session.Status.FromSide = &bgpv1alpha1.SessionFromSideStatus{
		MatchedProviders: int32(len(providerList.Items)),
		GeneratedPeers:   int32(len(expectedPeers)),
	}
	session.Status.ToSide = &bgpv1alpha1.SessionToSideStatus{
		PeerCount: int32(len(session.Spec.ToPeers)),
	}
	if err := r.Status().Patch(ctx, session, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch status: %w", err)
	}

	return ctrl.Result{}, nil
}

// reconcileExternalRef handles sessions that reference an external peer (no BGPPeer generated).
func (r *SessionReconciler) reconcileExternalRef(ctx context.Context, session *bgpv1alpha1.BGPSession) (reconcile.Result, error) {
	patch := client.MergeFrom(session.DeepCopy())
	apimeta.SetStatusCondition(&session.Status.Conditions, metav1.Condition{
		Type:               "PeersReconciled",
		Status:             metav1.ConditionTrue,
		Reason:             "ExternalPeerRef",
		Message:            fmt.Sprintf("session references external peer %q — no BGPPeer generated", session.Spec.FromExternalPeerRef.Name),
		ObservedGeneration: session.Generation,
	})
	if err := r.Status().Patch(ctx, session, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch status: %w", err)
	}
	return ctrl.Result{}, nil
}

// buildExpectedPeers computes the full set of BGPPeer resources that should exist.
func (r *SessionReconciler) buildExpectedPeers(
	ctx context.Context,
	session *bgpv1alpha1.BGPSession,
	providers []providersv1alpha1.BGPProvider,
) ([]*bgpv1alpha1.BGPPeer, error) {
	var expected []*bgpv1alpha1.BGPPeer

	for i := range providers {
		fromProvider := &providers[i]
		providerNodeIPv6 := r.nodeIPv6ForProvider(ctx, fromProvider)

		for _, toPeer := range session.Spec.ToPeers {
			// Skip self-peer.
			if providerNodeIPv6 != "" && toPeer.Address == providerNodeIPv6 {
				continue
			}

			peer := r.buildBGPPeer(session, fromProvider, toPeer)
			expected = append(expected, peer)
		}
	}

	return expected, nil
}

// buildBGPPeer constructs a BGPPeer resource from a BGPSession entry.
func (r *SessionReconciler) buildBGPPeer(
	session *bgpv1alpha1.BGPSession,
	fromProvider *providersv1alpha1.BGPProvider,
	toPeer bgpv1alpha1.SessionPeer,
) *bgpv1alpha1.BGPPeer {
	name := sanitizePeerName(session.Name + "-" + fromProvider.Name + "-" + toPeer.Address)

	instanceRef := session.Spec.FromInstanceRef
	if toPeer.InstanceRef != "" {
		instanceRef = toPeer.InstanceRef
	}

	return &bgpv1alpha1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				LabelManagedBy:   LabelManagedByManagement,
				LabelSessionName: session.Name,
			},
			Annotations: map[string]string{
				AnnotationSessionUID: string(session.UID),
			},
		},
		Spec: bgpv1alpha1.BGPPeerSpec{
			InstanceRef:          instanceRef,
			ProviderRef:          fromProvider.Name,
			Address:              toPeer.Address,
			ASNumber:             toPeer.ASNumber,
			AddressFamilies:      session.Spec.AddressFamilies,
			Timers:               session.Spec.Timers,
			AllowAsIn:            session.Spec.AllowAsIn,
			RouteReflectorClient: toPeer.RouteReflectorClient,
			RemotePort:           toPeer.RemotePort,
		},
	}
}

// sanitizePeerName truncates and sanitizes a string into a valid Kubernetes resource name.
func sanitizePeerName(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c)
		case c >= '0' && c <= '9':
			result = append(result, c)
		case c == '-' || c == '.':
			result = append(result, c)
		default:
			result = append(result, '-')
		}
	}
	// Trim trailing dashes.
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	if len(result) > 253 {
		result = result[:253]
	}
	return string(result)
}

// nodeIPv6ForProvider returns the IPv6 InternalIP of the node backing a BGPProvider.
// Returns empty string on error — self-peer check is best-effort.
func (r *SessionReconciler) nodeIPv6ForProvider(ctx context.Context, bp *providersv1alpha1.BGPProvider) string {
	nodeName := ""
	if bp.Annotations != nil {
		nodeName = bp.Annotations[LabelNode]
	}
	if nodeName == "" && bp.Labels != nil {
		nodeName = bp.Labels[LabelNode]
	}
	if nodeName == "" {
		return ""
	}

	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return ""
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeInternalIP {
			continue
		}
		ip := net.ParseIP(addr.Address)
		if ip == nil || ip.To4() != nil {
			continue
		}
		return addr.Address
	}
	return ""
}

// syncPeers creates missing peers, updates changed peers, and deletes stale peers.
func (r *SessionReconciler) syncPeers(
	ctx context.Context,
	expected []*bgpv1alpha1.BGPPeer,
	existing []bgpv1alpha1.BGPPeer,
) (created, updated, deleted int, err error) {
	existingByName := make(map[string]*bgpv1alpha1.BGPPeer, len(existing))
	for i := range existing {
		existingByName[existing[i].Name] = &existing[i]
	}

	expectedNames := make(map[string]struct{}, len(expected))
	for _, p := range expected {
		expectedNames[p.Name] = struct{}{}
	}

	for _, want := range expected {
		if got, exists := existingByName[want.Name]; !exists {
			if err := r.Create(ctx, want); err != nil {
				log.Printf("bgp/session: create BGPPeer %s: %v", want.Name, err)
			} else {
				created++
			}
		} else {
			if !bgpPeerSpecEqual(got.Spec, want.Spec) {
				patch := client.MergeFrom(got.DeepCopy())
				got.Spec = want.Spec
				if err := r.Patch(ctx, got, patch); err != nil {
					log.Printf("bgp/session: update BGPPeer %s: %v", want.Name, err)
				} else {
					updated++
				}
			}
		}
	}

	for _, p := range existing {
		if _, ok := expectedNames[p.Name]; ok {
			continue
		}
		if !hasSessionLabel(&p) {
			log.Printf("bgp/session: BGPPeer %s missing management labels — skipping GC", p.Name)
			continue
		}
		peer := p
		if err := r.Delete(ctx, &peer); err != nil {
			log.Printf("bgp/session: delete stale BGPPeer %s: %v", p.Name, err)
		} else {
			deleted++
		}
	}

	return created, updated, deleted, nil
}

// bgpPeerSpecEqual does a shallow comparison of key BGPPeerSpec fields.
func bgpPeerSpecEqual(a, b bgpv1alpha1.BGPPeerSpec) bool {
	return a.InstanceRef == b.InstanceRef &&
		a.ProviderRef == b.ProviderRef &&
		a.Address == b.Address &&
		a.ASNumber == b.ASNumber &&
		a.RouteReflectorClient == b.RouteReflectorClient
}

// hasSessionLabel returns true if the BGPPeer has the session-name management label.
func hasSessionLabel(p *bgpv1alpha1.BGPPeer) bool {
	if p.Labels == nil {
		return false
	}
	_, ok := p.Labels[LabelSessionName]
	return ok
}

// legacySessionFinalizer is the finalizer used by older versions of this controller.
// It is cleaned up alongside the current Finalizer to handle upgrade migrations.
const legacySessionFinalizer = "bgp.miloapis.com/session-cleanup"

// handleDelete removes all BGPPeer resources generated by this session.
func (r *SessionReconciler) handleDelete(ctx context.Context, session *bgpv1alpha1.BGPSession) error {
	hasNew := controllerutil.ContainsFinalizer(session, Finalizer)
	hasLegacy := controllerutil.ContainsFinalizer(session, legacySessionFinalizer)
	if !hasNew && !hasLegacy {
		return nil
	}

	var peerList bgpv1alpha1.BGPPeerList
	if err := r.List(ctx, &peerList, client.MatchingLabels{
		LabelSessionName: session.Name,
	}); err != nil {
		return fmt.Errorf("list BGPPeers: %w", err)
	}

	blocked := false
	for i := range peerList.Items {
		if err := r.Delete(ctx, &peerList.Items[i]); err != nil {
			log.Printf("bgp/session: delete BGPPeer %s: %v", peerList.Items[i].Name, err)
			blocked = true
		}
	}

	if blocked {
		return fmt.Errorf("deletion blocked: could not delete all generated BGPPeer resources")
	}

	patch := client.MergeFrom(session.DeepCopy())
	if hasNew {
		controllerutil.RemoveFinalizer(session, Finalizer)
	}
	if hasLegacy {
		controllerutil.RemoveFinalizer(session, legacySessionFinalizer)
	}
	return r.Patch(ctx, session, patch)
}

// SetupWithManager registers SessionReconciler with controller-runtime.
func (r *SessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPSession{}).
		Complete(r)
}
