# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

Cosmos defines Kubernetes CRDs and API types for BGP routing and virtual networking. It is **API-only** — no controllers, no binaries, no runtime. Implementations consume these APIs; this repo just defines the contract.

Module: `go.miloapis.com/cosmos`

## Commands

This project uses [Task](https://taskfile.dev/) (`task`), not `make`.

```bash
task tools        # Install all dev tools into ./bin/ (run first)
task build        # go fmt + go vet + go build ./...
task test:unit    # Run unit tests with coverage (excludes /e2e)
task test:e2e     # Create kind cluster, deploy CRDs, run Chainsaw tests, teardown
task lint         # golangci-lint + yamlfmt check
task lint-fix     # Same with auto-fix applied
task generate     # Regenerate zz_generated.deepcopy.go files
task manifests    # Regenerate CRD YAML in config/crd/ from Go types
task ci           # Full local pipeline: build → lint → test:unit → test:e2e
task clean        # Remove ./bin/ and cover.out
```

Run a single unit test package:
```bash
GOOS=linux go test ./api/bgp/v1alpha1/... -run TestName -v
```

All dev tools (golangci-lint, controller-gen, chainsaw, yamlfmt) are installed locally to `./bin/` — they are never installed system-wide.

## Architecture

### API groups

| Group              | Version    | Resources                                                       |
|--------------------|------------|-----------------------------------------------------------------|
| `bgp.miloapis.com` | `v1alpha1` | BGPRouter, BGPPeer, BGPAdvertisement, BGPPolicy, BGPVRFInstance |
| `vpc.miloapis.com` | `v1alpha1` | VPC, VPCAttachment                                              |

Source lives in `api/bgp/v1alpha1/` and `api/vpc/v1alpha1/`. Each resource has its own `*_types.go` file; shared types (RouterTarget, AddressFamily, etc.) live in `shared_types.go`.

### BGP resource ownership model

`BGPRouter` is the ownership boundary — one per routing plane per node. All other BGP resources bind to routers:

- `routerRef` — binds to a single BGPRouter by name (used by BGPPeer, BGPPolicy, BGPAdvertisement)
- `routerSelector` — binds to multiple BGPRouters by label (used by BGPPeer, BGPPolicy)
- `BGPAdvertisement` only supports `routerRef` (single-router scope)

The `RouterTarget` struct (in `shared_types.go`) is embedded by resources that support either binding. It carries a CEL validation rule enforcing exactly-one-of.

### Key invariants

- **ASN fields are `int64`** — `int32` produces an invalid OpenAPI v3 schema (max ASN > int32 max). Never use `int32` for ASN.
- **Kubernetes 1.28+ required** — CEL functions `isIP()` and `isCIDR()` are used for field validation.
- **Status conditions** follow `metav1.Condition` conventions. Condition type constants (e.g., `ConditionTypeReady`, `ConditionTypeAccepted`) are defined alongside the resource type they belong to.
- **YAML files must use `.yaml` extension**, never `.yml` — the lint task enforces this.

### Code generation

After changing kubebuilder markers (`// +kubebuilder:...`) or adding new types:

1. `task generate` — regenerates `zz_generated.deepcopy.go`
2. `task manifests` — regenerates CRDs in `config/crd/`

Both are generated; never edit `zz_generated.deepcopy.go` or CRD YAML directly.

### Testing

Unit tests (`api/**/*_types_test.go`) test validation, defaulting, and marshaling logic. E2E tests use [Chainsaw](https://kyverno.github.io/chainsaw/) against a dual-stack kind cluster (config at `test/e2e/kind-config.yaml`). The e2e suite is driven by `scripts/e2e.sh` and requires Docker.

## Architecture Reference

See [ARCHITECTURE.md](ARCHITECTURE.md) for a full architecture reference including module layout, package roles, resource model, data flow, and known constraints.

## Conventions Reference

See [CONVENTIONS.md](CONVENTIONS.md) for coding standards, naming rules, kubebuilder marker patterns, status condition conventions, and Go-specific conventions.

## Docs

- `docs/api/bgp.md` — full BGP CRD field reference
- `docs/api/vpc.md` — full VPC CRD field reference
- `docs/getting-started.md` — install and first resources
- `docs/enhancements/` — design proposals
