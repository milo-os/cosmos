package controller

import (
	"context"
	"errors"
	"fmt"
	"log"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bgpv1alpha1 "go.miloapis.com/bgp/api/v1alpha1"
	providersv1alpha1 "go.miloapis.com/bgp/api/providers/v1alpha1"
	"go.miloapis.com/bgp/internal/provider"
)

// AdvertisementReconciler reconciles BGPAdvertisement resources.
// It injects prefixes into the RIB via provider.AddOrUpdateAdvertisement.
//
// Active in: pop, infra.
type AdvertisementReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *provider.Registry
	NodeName string // from NODE_NAME env var
}

// Reconcile handles BGPAdvertisement events.
func (r *AdvertisementReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var adv bgpv1alpha1.BGPAdvertisement
	if err := r.Get(ctx, req.NamespacedName, &adv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !adv.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &adv)
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&adv, Finalizer) {
		patch := client.MergeFrom(adv.DeepCopy())
		controllerutil.AddFinalizer(&adv, Finalizer)
		if err := r.Patch(ctx, &adv, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &adv); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Resolve BGPInstance.
	var instance bgpv1alpha1.BGPInstance
	if err := r.Get(ctx, types.NamespacedName{Name: adv.Spec.InstanceRef}, &instance); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return r.setAdvCondition(ctx, &adv, "InstanceNotFound", metav1.ConditionTrue,
			"InstanceNotFound", fmt.Sprintf("BGPInstance %q not found", adv.Spec.InstanceRef))
	}

	// Validate that the BGPInstance has Unicast address families.
	if !hasUnicastFamily(instance.Spec.AddressFamilies) {
		return r.setAdvCondition(ctx, &adv, "UnsupportedAddressFamily", metav1.ConditionTrue,
			"UnsupportedAddressFamily", "referenced BGPInstance has no Unicast address family")
	}

	// List providers matching the BGPInstance's providerSelector.
	sel, err := metav1.LabelSelectorAsSelector(&instance.Spec.ProviderSelector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid providerSelector: %w", err)
	}

	var providerList providersv1alpha1.BGPProviderList
	if err := r.List(ctx, &providerList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list BGPProviders: %w", err)
	}

	for i := range providerList.Items {
		bp := &providerList.Items[i]
		if err := r.reconcileForProvider(ctx, &adv, bp); err != nil {
			return ctrl.Result{}, fmt.Errorf("provider %s: %w", bp.Name, err)
		}
	}
	return ctrl.Result{}, nil
}

// reconcileForProvider injects prefixes on one provider.
func (r *AdvertisementReconciler) reconcileForProvider(
	ctx context.Context,
	adv *bgpv1alpha1.BGPAdvertisement,
	bp *providersv1alpha1.BGPProvider,
) error {
	impl, ok := r.Registry.Get(bp.Name)
	if !ok {
		if r.NodeName != "" && bp.Labels[LabelNode] != r.NodeName {
			return nil
		}
		return r.writeAdvProviderStatus(ctx, adv, bp.Name, bp.Spec.Type, false,
			"DaemonUnavailable", "provider not in registry — daemon may be starting")
	}

	// Resolve peer addresses from peerSelector if set.
	var peerAddresses []string
	if adv.Spec.PeerSelector != nil {
		peerSel, err := metav1.LabelSelectorAsSelector(adv.Spec.PeerSelector)
		if err != nil {
			return fmt.Errorf("invalid peerSelector: %w", err)
		}
		var peerList bgpv1alpha1.BGPPeerList
		if err := r.List(ctx, &peerList, &client.ListOptions{LabelSelector: peerSel}); err != nil {
			return fmt.Errorf("list BGPPeers: %w", err)
		}
		for _, p := range peerList.Items {
			peerAddresses = append(peerAddresses, p.Spec.Address)
		}
	}

	advSpec := provider.AdvertisementSpec{
		Prefixes:      adv.Spec.Prefixes,
		PeerAddresses: peerAddresses,
	}

	if err := impl.AddOrUpdateAdvertisement(ctx, advSpec); err != nil {
		return r.writeAdvProviderStatus(ctx, adv, bp.Name, bp.Spec.Type, false,
			"AdvertisementFailed", fmt.Sprintf("AddOrUpdateAdvertisement: %v", err))
	}

	RecordAdvertisedPrefixes(adv.Name, len(adv.Spec.Prefixes))
	return r.writeAdvProviderStatus(ctx, adv, bp.Name, bp.Spec.Type, true,
		"Advertised",
		fmt.Sprintf("%d prefix(es) advertised", len(adv.Spec.Prefixes)))
}

// hasUnicastFamily returns true when any address family has SAFI Unicast.
func hasUnicastFamily(afs []bgpv1alpha1.AddressFamily) bool {
	for _, af := range afs {
		if af.SAFI == bgpv1alpha1.SAFIUnicast {
			return true
		}
	}
	return false
}

// writeAdvProviderStatus writes per-provider status for a BGPAdvertisement.
func (r *AdvertisementReconciler) writeAdvProviderStatus(
	ctx context.Context,
	adv *bgpv1alpha1.BGPAdvertisement,
	providerName, daemonType string,
	ok bool,
	reason, msg string,
) error {
	condStatus := metav1.ConditionFalse
	if ok {
		condStatus = metav1.ConditionTrue
	}
	cond := metav1.Condition{
		Type:               "Advertised",
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: adv.Generation,
	}

	updated := adv.DeepCopy()
	found := false
	for i, ps := range updated.Status.Providers {
		if ps.ProviderName == providerName {
			apimeta.SetStatusCondition(&updated.Status.Providers[i].Conditions, cond)
			if ok {
				if updated.Status.Providers[i].ResolvedConfig == nil {
					updated.Status.Providers[i].ResolvedConfig = &bgpv1alpha1.ResolvedProviderConfig{}
				}
				updated.Status.Providers[i].ResolvedConfig.ResolvedPrefixes = adv.Spec.Prefixes
			}
			found = true
			break
		}
	}
	if !found {
		ps := bgpv1alpha1.ProviderStatus{
			ProviderName: providerName,
			Daemon:       daemonType,
		}
		if ok {
			ps.ResolvedConfig = &bgpv1alpha1.ResolvedProviderConfig{
				ResolvedPrefixes: adv.Spec.Prefixes,
			}
		}
		updated.Status.Providers = append(updated.Status.Providers, ps)
		apimeta.SetStatusCondition(&updated.Status.Providers[len(updated.Status.Providers)-1].Conditions, cond)
	}

	patch := client.MergeFrom(adv)
	patchErr := r.Status().Patch(ctx, updated, patch)
	if !ok {
		return errors.Join(fmt.Errorf("%s: %s", reason, msg), patchErr)
	}
	return patchErr
}

// setAdvCondition writes a top-level status condition and returns an error so
// the controller re-queues with backoff until the underlying issue is resolved.
func (r *AdvertisementReconciler) setAdvCondition(
	ctx context.Context,
	adv *bgpv1alpha1.BGPAdvertisement,
	condType string,
	condStatus metav1.ConditionStatus,
	reason, msg string,
) (reconcile.Result, error) {
	updated := adv.DeepCopy()
	apimeta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: adv.Generation,
	})
	if err := r.Status().Patch(ctx, updated, client.MergeFrom(adv)); err != nil {
		log.Printf("bgp/adv: %s patch condition %s: %v", adv.Name, reason, err)
	}
	return ctrl.Result{}, fmt.Errorf("%s: %s", reason, msg)
}

// handleDelete withdraws all prefixes on each provider and removes the finalizer.
func (r *AdvertisementReconciler) handleDelete(ctx context.Context, adv *bgpv1alpha1.BGPAdvertisement) error {
	if !controllerutil.ContainsFinalizer(adv, Finalizer) {
		return nil
	}

	// Best-effort: withdraw from all known providers.
	for _, ps := range adv.Status.Providers {
		impl, ok := r.Registry.Get(ps.ProviderName)
		if !ok {
			continue
		}
		for _, prefix := range adv.Spec.Prefixes {
			if err := impl.DeleteAdvertisement(ctx, prefix); err != nil {
				log.Printf("bgp/adv: delete prefix %s on provider %s: %v", prefix, ps.ProviderName, err)
			}
		}
	}

	RecordAdvertisedPrefixes(adv.Name, 0)

	patch := client.MergeFrom(adv.DeepCopy())
	controllerutil.RemoveFinalizer(adv, Finalizer)
	return r.Patch(ctx, adv, patch)
}

// mapInstanceToAdvertisements re-triggers reconciliation for all BGPAdvertisements
// that reference a changed BGPInstance, so advertisements react when their instance appears.
func (r *AdvertisementReconciler) mapInstanceToAdvertisements(ctx context.Context, obj client.Object) []reconcile.Request {
	instance, ok := obj.(*bgpv1alpha1.BGPInstance)
	if !ok {
		return nil
	}
	var advList bgpv1alpha1.BGPAdvertisementList
	if err := r.List(ctx, &advList); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, adv := range advList.Items {
		if adv.Spec.InstanceRef == instance.Name {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: adv.Name}})
		}
	}
	return reqs
}

// SetupWithManager registers AdvertisementReconciler with controller-runtime.
func (r *AdvertisementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPAdvertisement{}).
		Watches(
			&bgpv1alpha1.BGPInstance{},
			handler.EnqueueRequestsFromMapFunc(r.mapInstanceToAdvertisements),
		).
		Complete(r)
}
