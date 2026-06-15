package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
)

// ExternalPeerReconciler reconciles BGPExternalPeer resources.
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
	return ctrl.Result{}, nil
}

// SetupWithManager registers ExternalPeerReconciler with controller-runtime.
func (r *ExternalPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPExternalPeer{}).
		Complete(r)
}
