// Package controller implements the Kubernetes-native BGP control plane.
// It reconciles BGP CRDs into provider calls via the provider.Registry abstraction.
package controller

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	bgpv1alpha1 "go.miloapis.com/cosmos/api/bgp/v1alpha1"
	providersv1alpha1 "go.miloapis.com/cosmos/api/providers/v1alpha1"
	"go.miloapis.com/cosmos/internal/provider"
)

// scheme holds the runtime.Scheme for all types used by the manager.
var scheme = runtime.NewScheme()

// Scheme returns the controller's runtime.Scheme, pre-populated with all API types.
// Exported for use by the CLI startup code to build a direct (non-cached) k8s client.
func Scheme() *runtime.Scheme {
	return scheme
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(bgpv1alpha1.AddToScheme(scheme))
	utilruntime.Must(providersv1alpha1.AddToScheme(scheme))
}

// Manager holds the shared configuration for all BGP reconcilers.
// It wires reconcilers based on ClusterRole at startup.
type Manager struct {
	// ClusterRole is one of "pop", "infra", or "management".
	ClusterRole string
	// Registry is the shared provider registry. All reconcilers look up provider
	// implementations here. ProviderReconciler populates it at runtime.
	Registry *provider.Registry
	// NodeName is the Kubernetes node name for this pod. Used by ProviderReconciler
	// to scope reconciliation to BGPProvider resources on this node.
	NodeName    string
	MetricsAddr string
	HealthAddr  string
}

// SetupWithManager registers all reconcilers appropriate for m.ClusterRole.
func (m *Manager) SetupWithManager(mgr ctrl.Manager) error {
	isPOPOrInfra := m.ClusterRole == "pop" || m.ClusterRole == "infra"
	isManagement := m.ClusterRole == "management"

	// ProviderReconciler: active in pop and infra.
	if isPOPOrInfra {
		providerReconciler := &ProviderReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Registry: m.Registry,
			NodeName: m.NodeName,
		}
		if err := providerReconciler.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup ProviderReconciler: %w", err)
		}
	}

	// InstanceReconciler: active in pop and infra.
	if isPOPOrInfra {
		if err := (&InstanceReconciler{
			Client:      mgr.GetClient(),
			Scheme:      mgr.GetScheme(),
			Registry:    m.Registry,
			ClusterRole: m.ClusterRole,
			NodeName:    m.NodeName,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup InstanceReconciler: %w", err)
		}
	}

	// PeerReconciler: active in pop and infra.
	if isPOPOrInfra {
		if err := (&PeerReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Registry: m.Registry,
			NodeName: m.NodeName,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup PeerReconciler: %w", err)
		}
	}

	// AdvertisementReconciler: active in pop and infra.
	if isPOPOrInfra {
		if err := (&AdvertisementReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Registry: m.Registry,
			NodeName: m.NodeName,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup AdvertisementReconciler: %w", err)
		}
	}

	// RoutePolicyReconciler: active in pop and infra.
	if isPOPOrInfra {
		if err := (&RoutePolicyReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Registry: m.Registry,
			NodeName: m.NodeName,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup RoutePolicyReconciler: %w", err)
		}
	}

	// SessionReconciler: active in all cluster roles.
	if err := (&SessionReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ClusterRole: m.ClusterRole,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup SessionReconciler: %w", err)
	}

	// ExternalPeerReconciler: active in management.
	if isManagement {
		if err := (&ExternalPeerReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup ExternalPeerReconciler: %w", err)
		}
	}

	return nil
}

// Run starts the BGP CRD controller and blocks until ctx is cancelled.
// restCfg is the Kubernetes REST config. clusterRole must be "pop", "infra", or "management".
func Run(ctx context.Context, metricsAddr, healthAddr, clusterRole, nodeName string) error {
	if metricsAddr == "" {
		metricsAddr = ":8082"
	}
	if healthAddr == "" {
		healthAddr = ":8083"
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get k8s config: %w", err)
	}

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: healthAddr,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
	})
	if err != nil {
		return fmt.Errorf("new manager: %w", err)
	}

	m := &Manager{
		ClusterRole: clusterRole,
		Registry:    provider.NewRegistry(),
		NodeName:    nodeName,
		MetricsAddr: metricsAddr,
		HealthAddr:  healthAddr,
	}

	if err := m.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup reconcilers: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	log.Printf("bgp/controller: starting manager (clusterRole=%s node=%s)", clusterRole, nodeName)
	return mgr.Start(ctx)
}
