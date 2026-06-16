package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// advertisedPrefixesGauge tracks the number of prefixes advertised per BGPAdvertisement.
	advertisedPrefixesGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bgp_advertised_prefixes_total",
			Help: "Number of prefixes advertised from a BGPAdvertisement resource",
		},
		[]string{"advertisement"},
	)

	// routePoliciesAppliedGauge tracks whether a BGPRoutePolicy is successfully applied.
	routePoliciesAppliedGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bgp_route_policies_applied",
			Help: "1 if the BGPRoutePolicy is applied to the provider, 0 otherwise",
		},
		[]string{"policy"},
	)
)

func init() {
	// Register metrics with the controller-runtime metrics registry so they
	// are exposed on the manager's /metrics endpoint.
	metrics.Registry.MustRegister(
		advertisedPrefixesGauge,
		routePoliciesAppliedGauge,
	)
}

// RecordAdvertisedPrefixes sets the advertised-prefixes gauge for a BGPAdvertisement.
func RecordAdvertisedPrefixes(advertisementName string, count int) {
	advertisedPrefixesGauge.WithLabelValues(advertisementName).Set(float64(count))
}

// RecordRoutePolicyApplied sets the route-policy-applied gauge to 1 (applied) or 0 (not applied).
func RecordRoutePolicyApplied(policyName string, applied bool) {
	v := 0.0
	if applied {
		v = 1.0
	}
	routePoliciesAppliedGauge.WithLabelValues(policyName).Set(v)
}
