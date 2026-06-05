/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	bgpcontroller "go.miloapis.com/bgp/internal/controller"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	metricsAddr string
	healthAddr  string
	clusterRole string
}

func newRootCommand() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   "bgp",
		Short: "BGP operator — reconciles BGP CRDs against local BGP daemons",
		Long: `The BGP operator reconciles BGP CRDs against independently-running BGP daemons.
It reads its cluster role from the cosmos-config ConfigMap in cosmos-system and
reconciles BGPProvider, BGPInstance, BGPPeer, BGPAdvertisement, BGPRoutePolicy,
BGPSession, and BGPExternalPeer CRDs. BGPProvider resources specify the gRPC
endpoint of a pre-existing daemon (FRR or GoBGP) that cosmos connects to.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.metricsAddr, "metrics-addr", ":8082", "Address to serve Prometheus metrics on")
	cmd.Flags().StringVar(&opts.healthAddr, "health-addr", ":8083", "Address to serve health/readiness probes on")
	cmd.Flags().StringVar(&opts.clusterRole, "cluster-role", "", "Override cluster role (pop|infra|management); skips cosmos-config ConfigMap lookup when set")

	return cmd
}

// validClusterRoles is the set of accepted clusterRole values in cosmos-config.
var validClusterRoles = map[string]bool{
	"pop":        true,
	"infra":      true,
	"management": true,
}

func run(ctx context.Context, opts *options) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Read NODE_NAME from Downward API environment variable.
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Printf("bgp: NODE_NAME not set — provider auto-bootstrap disabled")
	}

	// Resolve clusterRole: use the flag value directly when set; otherwise read from
	// the cosmos-config ConfigMap in cosmos-system (production default).
	clusterRole := opts.clusterRole
	if clusterRole == "" {
		var err error
		clusterRole, err = readClusterRole(ctx)
		if err != nil {
			return fmt.Errorf("read cluster role: %w", err)
		}
	} else if !validClusterRoles[clusterRole] {
		return fmt.Errorf("invalid --cluster-role %q: must be one of pop, infra, management", clusterRole)
	}

	log.Printf("bgp: starting (clusterRole=%s node=%s)", clusterRole, nodeName)

	return bgpcontroller.Run(ctx, opts.metricsAddr, opts.healthAddr, clusterRole, nodeName)
}

// readClusterRole reads the clusterRole field from the cosmos-config ConfigMap
// in the cosmos-system namespace. Returns an error if the ConfigMap is missing,
// the key is absent, or the value is not one of: pop, infra, management.
func readClusterRole(ctx context.Context) (string, error) {
	restCfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return "", fmt.Errorf("get k8s config: %w", err)
	}

	c, err := client.New(restCfg, client.Options{Scheme: bgpcontroller.Scheme()})
	if err != nil {
		return "", fmt.Errorf("create k8s client: %w", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: "cosmos-system",
		Name:      "cosmos-config",
	}, &cm); err != nil {
		return "", fmt.Errorf("get cosmos-config ConfigMap from cosmos-system: %w", err)
	}

	role, ok := cm.Data["clusterRole"]
	if !ok {
		return "", fmt.Errorf("cosmos-config ConfigMap is missing the 'clusterRole' key")
	}
	if !validClusterRoles[role] {
		return "", fmt.Errorf("invalid clusterRole %q: must be one of pop, infra, management", role)
	}

	return role, nil
}
