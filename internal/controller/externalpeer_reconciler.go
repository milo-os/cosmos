package controller

import (
	"context"
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bgpv1alpha1 "go.miloapis.com/bgp/api/bgp/v1alpha1"
)

// ExternalPeerReconciler reconciles BGPExternalPeer resources.
// It tracks how many BGPSession resources reference each external peer and
// blocks deletion when the count is non-zero.
//
// Active in: management.
type ExternalPeerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile handles BGPExternalPeer events.
func (r *ExternalPeerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var ep bgpv1alpha1.BGPExternalPeer
	if err := r.Get(ctx, req.NamespacedName, &ep); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !ep.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &ep)
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&ep, Finalizer) {
		patch := client.MergeFrom(ep.DeepCopy())
		controllerutil.AddFinalizer(&ep, Finalizer)
		if err := r.Patch(ctx, &ep, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &ep); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Count referencing BGPSession resources.
	count, names, err := r.countReferences(ctx, ep.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("count references: %w", err)
	}

	// Update status.
	patch := client.MergeFrom(ep.DeepCopy())
	ep.Status.ReferencedBy = int32(count)
	ep.Status.ReferencedByList = cappedNames(names, 50)

	inUse := metav1.ConditionFalse
	inUseMsg := "no BGPSession resources reference this external peer"
	if count > 0 {
		inUse = metav1.ConditionTrue
		inUseMsg = fmt.Sprintf("%d BGPSession resource(s) reference this external peer", count)
	}
	apimeta.SetStatusCondition(&ep.Status.Conditions, metav1.Condition{
		Type:               "InUse",
		Status:             inUse,
		Reason:             "ReferenceCount",
		Message:            inUseMsg,
		ObservedGeneration: ep.Generation,
	})

	if err := r.Status().Patch(ctx, &ep, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch status: %w", err)
	}

	return ctrl.Result{}, nil
}

// countReferences counts how many BGPSession resources reference this external peer.
func (r *ExternalPeerReconciler) countReferences(ctx context.Context, epName string) (int, []string, error) {
	var sessionList bgpv1alpha1.BGPSessionList
	if err := r.List(ctx, &sessionList); err != nil {
		return 0, nil, fmt.Errorf("list BGPSessions: %w", err)
	}

	var names []string
	for _, s := range sessionList.Items {
		if s.Spec.FromExternalPeerRef != nil && s.Spec.FromExternalPeerRef.Name == epName {
			names = append(names, s.Name)
		}
	}
	return len(names), names, nil
}

// cappedNames returns up to maxLen entries from names.
func cappedNames(names []string, maxLen int) []string {
	if len(names) <= maxLen {
		return names
	}
	return names[:maxLen]
}

// handleDelete blocks deletion when in-use and removes the finalizer when safe.
func (r *ExternalPeerReconciler) handleDelete(ctx context.Context, ep *bgpv1alpha1.BGPExternalPeer) error {
	if !controllerutil.ContainsFinalizer(ep, Finalizer) {
		return nil
	}

	count, _, err := r.countReferences(ctx, ep.Name)
	if err != nil {
		return fmt.Errorf("count references: %w", err)
	}

	if count > 0 {
		patch := client.MergeFrom(ep.DeepCopy())
		apimeta.SetStatusCondition(&ep.Status.Conditions, metav1.Condition{
			Type:               "DeletionBlocked",
			Status:             metav1.ConditionTrue,
			Reason:             "InUse",
			Message:            fmt.Sprintf("%d BGPSession resource(s) still reference this external peer", count),
			ObservedGeneration: ep.Generation,
		})
		_ = r.Status().Patch(ctx, ep, patch)
		return fmt.Errorf("deletion blocked: %d BGPSession resource(s) still reference this external peer", count)
	}

	patch := client.MergeFrom(ep.DeepCopy())
	controllerutil.RemoveFinalizer(ep, Finalizer)
	return r.Patch(ctx, ep, patch)
}

// mapSessionToExternalPeers re-triggers ExternalPeer reconciliation when a BGPSession changes.
func (r *ExternalPeerReconciler) mapSessionToExternalPeers(ctx context.Context, obj client.Object) []reconcile.Request {
	session, ok := obj.(*bgpv1alpha1.BGPSession)
	if !ok {
		return nil
	}
	if session.Spec.FromExternalPeerRef == nil {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: session.Spec.FromExternalPeerRef.Name}},
	}
}

// SetupWithManager registers ExternalPeerReconciler with controller-runtime.
// It also watches BGPSession changes to re-trigger reconciliation when sessions
// reference or release external peers.
func (r *ExternalPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPExternalPeer{}).
		Watches(
			&bgpv1alpha1.BGPSession{},
			handler.EnqueueRequestsFromMapFunc(r.mapSessionToExternalPeers),
		).
		Complete(r)
}
