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

	bgpcontroller "go.miloapis.com/cosmos/internal/controller"
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
}

func newRootCommand() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   "cosmos-operator",
		Short: "Cosmos operator — reconciles network CRDs against local BGP daemons",
		Long: `Cosmos reconciles network CRDs against independently-running BGP daemons.
It reconciles BGPProvider, BGPInstance, BGPPeer, BGPAdvertisement, BGPRoutePolicy,
and BGPExternalPeer CRDs. BGPProvider resources specify the gRPC endpoint of a
remote BGP agent that implements the BGPProviderService proto interface.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.metricsAddr, "metrics-addr", ":8082", "Address to serve Prometheus metrics on")
	cmd.Flags().StringVar(&opts.healthAddr, "health-addr", ":8083", "Address to serve health/readiness probes on")

	return cmd
}

func run(ctx context.Context, opts *options) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Printf("cosmos: NODE_NAME not set — provider auto-bootstrap disabled")
	}

	log.Printf("cosmos: starting (node=%s)", nodeName)

	return bgpcontroller.Run(ctx, opts.metricsAddr, opts.healthAddr, nodeName)
}
