package controller

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bgpv1alpha1 "go.miloapis.com/bgp/api/v1alpha1"
	providersv1alpha1 "go.miloapis.com/bgp/api/providers/v1alpha1"
	"go.miloapis.com/bgp/internal/provider"
)

// RoutePolicyReconciler reconciles BGPRoutePolicy resources.
// It resolves peer selectors, orders all policies by priority, and programs them
// on each matched provider.
//
// Active in: pop, infra.
type RoutePolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *provider.Registry
	NodeName string // from NODE_NAME env var
}

// Reconcile handles BGPRoutePolicy events.
func (r *RoutePolicyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var pol bgpv1alpha1.BGPRoutePolicy
	if err := r.Get(ctx, req.NamespacedName, &pol); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !pol.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleDelete(ctx, &pol)
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(&pol, Finalizer) {
		patch := client.MergeFrom(pol.DeepCopy())
		controllerutil.AddFinalizer(&pol, Finalizer)
		if err := r.Patch(ctx, &pol, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		if err := r.Get(ctx, req.NamespacedName, &pol); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Resolve BGPInstance.
	var instance bgpv1alpha1.BGPInstance
	if err := r.Get(ctx, types.NamespacedName{Name: pol.Spec.InstanceRef}, &instance); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		log.Printf("bgp/routepolicy: %s instance %s not found", pol.Name, pol.Spec.InstanceRef)
		return ctrl.Result{}, nil
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

	// Resolve matched BGPPeer resources via peerSelector (if set).
	var matchedPeers []bgpv1alpha1.BGPPeer
	noMatchingPeers := false
	if pol.Spec.PeerSelector != nil {
		peerSel, err := metav1.LabelSelectorAsSelector(pol.Spec.PeerSelector)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid peerSelector: %w", err)
		}
		var peerList bgpv1alpha1.BGPPeerList
		if err := r.List(ctx, &peerList, &client.ListOptions{LabelSelector: peerSel}); err != nil {
			return ctrl.Result{}, fmt.Errorf("list BGPPeers: %w", err)
		}
		matchedPeers = peerList.Items
		if len(matchedPeers) == 0 {
			noMatchingPeers = true
		}
	}

	if noMatchingPeers {
		log.Printf("bgp/routepolicy: %s peerSelector matched no BGPPeer resources", pol.Name)
	}

	// Collect all BGPRoutePolicies for this instance and sort by priority (desc), then name (asc).
	var allPolicies bgpv1alpha1.BGPRoutePolicyList
	if err := r.List(ctx, &allPolicies); err != nil {
		return ctrl.Result{}, fmt.Errorf("list all BGPRoutePolicies: %w", err)
	}
	orderedPolicies := policiesForInstance(allPolicies.Items, pol.Spec.InstanceRef)
	sortPolicies(orderedPolicies)

	// Find this policy's position in the ordered list to derive its effective policy name.
	var needsRequeue bool
	for i := range providerList.Items {
		bp := &providerList.Items[i]
		if err := r.reconcileForProvider(ctx, &pol, bp, orderedPolicies, matchedPeers); err != nil {
			log.Printf("bgp/routepolicy: %s provider %s: %v", pol.Name, bp.Name, err)
			needsRequeue = true
		}
	}

	if needsRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// reconcileForProvider applies a route policy to one provider.
func (r *RoutePolicyReconciler) reconcileForProvider(
	ctx context.Context,
	pol *bgpv1alpha1.BGPRoutePolicy,
	bp *providersv1alpha1.BGPProvider,
	_ []bgpv1alpha1.BGPRoutePolicy,
	matchedPeers []bgpv1alpha1.BGPPeer,
) error {
	impl, ok := r.Registry.Get(bp.Name)
	if !ok {
		if r.NodeName != "" && bp.Labels[LabelNode] != r.NodeName {
			return nil
		}
		return r.writePolicyProviderStatus(ctx, pol, bp.Name, bp.Spec.Type, false,
			"DaemonUnavailable", "provider not in registry — daemon may be starting")
	}

	// Build peer addresses for the policy spec.
	var peerAddresses []string
	for _, p := range matchedPeers {
		peerAddresses = append(peerAddresses, p.Spec.Address)
	}

	// Build PolicySpec from the BGPRoutePolicy.
	policySpec := buildPolicySpec(pol, peerAddresses)

	if err := impl.AddOrUpdatePolicy(ctx, policySpec); err != nil {
		return r.writePolicyProviderStatus(ctx, pol, bp.Name, bp.Spec.Type, false,
			"PolicyApplicationFailed", fmt.Sprintf("AddOrUpdatePolicy: %v", err))
	}

	RecordRoutePolicyApplied(pol.Name, true)
	return r.writePolicyProviderStatus(ctx, pol, bp.Name, bp.Spec.Type, true,
		"Applied", fmt.Sprintf("policy %s applied", pol.Name))
}

// buildPolicySpec converts BGPRoutePolicy to provider.PolicySpec.
func buildPolicySpec(pol *bgpv1alpha1.BGPRoutePolicy, peerAddresses []string) provider.PolicySpec {
	importStmts := convertPolicyStatements(pol.Spec.ImportStatements)
	exportStmts := convertPolicyStatements(pol.Spec.ExportStatements)

	return provider.PolicySpec{
		Name:             pol.Name,
		Priority:         pol.Spec.Priority,
		ImportStatements: importStmts,
		ExportStatements: exportStmts,
	}
}

// convertPolicyStatements converts API policy statements to provider policy statements.
func convertPolicyStatements(stmts []bgpv1alpha1.PolicyStatement) []provider.PolicyStatement {
	result := make([]provider.PolicyStatement, 0, len(stmts))
	for _, s := range stmts {
		ps := provider.PolicyStatement{
			Name: s.Name,
			Actions: provider.PolicyActions{
				RouteDisposition: s.Actions.RouteDisposition,
				SetNextHop:       s.Actions.SetNextHop,
			},
		}
		if s.Actions.SetLocalPreference != nil {
			v := *s.Actions.SetLocalPreference
			ps.Actions.SetLocalPreference = &v
		}
		if s.Actions.SetMED != nil {
			v := *s.Actions.SetMED
			ps.Actions.SetMED = &v
		}
		if s.Actions.SetCommunity != nil {
			ps.Actions.SetCommunity = &provider.SetCommunityAction{
				Communities: s.Actions.SetCommunity.Communities,
				Method:      s.Actions.SetCommunity.Method,
			}
		}
		if s.Conditions != nil {
			ps.Conditions = &provider.PolicyConditions{
				PrefixSets:   s.Conditions.PrefixSets,
				CommunitySet: s.Conditions.CommunitySet,
				NextHopSet:   s.Conditions.NextHopSet,
			}
		}
		result = append(result, ps)
	}
	return result
}

// policiesForInstance filters policies by instanceRef.
func policiesForInstance(all []bgpv1alpha1.BGPRoutePolicy, instanceRef string) []bgpv1alpha1.BGPRoutePolicy {
	var result []bgpv1alpha1.BGPRoutePolicy
	for _, p := range all {
		if p.Spec.InstanceRef == instanceRef {
			result = append(result, p)
		}
	}
	return result
}

// sortPolicies sorts by priority descending, then by name ascending.
func sortPolicies(policies []bgpv1alpha1.BGPRoutePolicy) {
	sort.Slice(policies, func(i, j int) bool {
		if policies[i].Spec.Priority != policies[j].Spec.Priority {
			return policies[i].Spec.Priority > policies[j].Spec.Priority
		}
		return policies[i].Name < policies[j].Name
	})
}

// writePolicyProviderStatus writes per-provider status for a BGPRoutePolicy.
func (r *RoutePolicyReconciler) writePolicyProviderStatus(
	ctx context.Context,
	pol *bgpv1alpha1.BGPRoutePolicy,
	providerName, daemonType string,
	applied bool,
	reason, msg string,
) error {
	condStatus := metav1.ConditionFalse
	if applied {
		condStatus = metav1.ConditionTrue
	}
	cond := metav1.Condition{
		Type:               "Applied",
		Status:             condStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: pol.Generation,
	}

	updated := pol.DeepCopy()
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

	patch := client.MergeFrom(pol)
	if err := r.Status().Patch(ctx, updated, patch); err != nil {
		log.Printf("bgp/routepolicy: patch status for provider %s: %v", providerName, err)
	}

	if !applied {
		return fmt.Errorf("%s: %s", reason, msg)
	}
	return nil
}

// handleDelete removes policies from all providers and removes the finalizer.
func (r *RoutePolicyReconciler) handleDelete(ctx context.Context, pol *bgpv1alpha1.BGPRoutePolicy) error {
	if !controllerutil.ContainsFinalizer(pol, Finalizer) {
		return nil
	}

	blocked := false
	for _, ps := range pol.Status.Providers {
		impl, ok := r.Registry.Get(ps.ProviderName)
		if !ok {
			continue
		}
		if err := impl.DeletePolicy(ctx, pol.Name); err != nil {
			log.Printf("bgp/routepolicy: delete policy %s on provider %s: %v", pol.Name, ps.ProviderName, err)
			blocked = true
		}
	}

	if blocked {
		return fmt.Errorf("deletion blocked: daemon unavailable for one or more providers")
	}

	RecordRoutePolicyApplied(pol.Name, false)

	patch := client.MergeFrom(pol.DeepCopy())
	controllerutil.RemoveFinalizer(pol, Finalizer)
	return r.Patch(ctx, pol, patch)
}

// SetupWithManager registers RoutePolicyReconciler with controller-runtime.
func (r *RoutePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bgpv1alpha1.BGPRoutePolicy{}).
		Complete(r)
}
