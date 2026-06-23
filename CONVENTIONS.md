# Conventions

> Cosmos is a Go module defining Kubernetes CRDs and API types for BGP routing and virtual networking. It is API-only — no controllers, no runtime code.

_Last updated: 2026-06-23_

---

## Table of Contents

1. [Core Conventions](#core-conventions)
   - [Naming](#naming)
   - [Code Organization](#code-organization)
   - [Comments and Documentation](#comments-and-documentation)
   - [Testing](#testing)
   - [Dependencies and Imports](#dependencies-and-imports)
   - [Git and Version Control](#git-and-version-control)
2. [Go Conventions](#go-conventions)
   - [Module and Package Layout](#module-and-package-layout)
   - [Type Naming](#type-naming)
   - [kubebuilder Markers](#kubebuilder-markers)
   - [Status Conditions](#status-conditions)
   - [Field Types and Invariants](#field-types-and-invariants)
   - [JSON Serialization](#json-serialization)
   - [Code Generation](#code-generation)
   - [Error Handling](#error-handling)
   - [Testing](#go-testing)
   - [Linting and Formatting](#linting-and-formatting)
3. [YAML Conventions](#yaml-conventions)
4. [Markdown Conventions](#markdown-conventions)
5. [For Claude](#for-claude)

---

## Core Conventions

### Naming

**Source files** use `snake_case` with a `_types.go` suffix for CRD type definitions:

```
router_types.go         ← BGPRouter CRD
peer_types.go           ← BGPPeer CRD
shared_types.go         ← types shared across resources in the package
groupversion_info.go    ← scheme registration (fixed name)
```

**Test files** use `_test.go` suffix adjacent to the file under test:

```
router_types_test.go    ← tests for router_types.go
```

**Generated files** use the `zz_generated.` prefix. Never edit them:

```
zz_generated.deepcopy.go
```

**Directories** use lowercase kebab-case. The version is part of the path (`v1alpha1`).

### Code Organization

All API types live under `api/<group>/<version>/`. One `_types.go` file per CRD resource. Types shared across resources in the same package live in `shared_types.go`.

```
api/
  bgp/v1alpha1/         ← bgp.miloapis.com API group
  vpc/v1alpha1/         ← vpc.miloapis.com API group
config/crd/             ← generated CRD YAML — never edit directly
config/samples/         ← example resources
docs/api/               ← human-readable field reference
docs/enhancements/      ← design proposals
hack/                   ← code generation support files
scripts/                ← shell scripts (e2e orchestration)
test/e2e/               ← Chainsaw e2e tests
bin/                    ← local dev tools (gitignored)
```

### Comments and Documentation

**Exported types** require a godoc comment that begins with the type name and ends with a period:

```go
// BGPRouter defines a logical BGP routing context. It abstracts a processing
// instance bound to a specific execution context.
type BGPRouter struct { ... }
```

**Spec and Status types** follow the "defines the desired/observed state of..." convention:

```go
// BGPRouterSpec defines the desired state of a BGPRouter.
type BGPRouterSpec struct { ... }

// BGPRouterStatus defines the observed state of a BGPRouter.
type BGPRouterStatus struct { ... }
```

**Enum constants** each get a godoc comment:

```go
// BGPPolicyActionPermit allows the route and optionally applies set actions.
BGPPolicyActionPermit BGPPolicyAction = "permit"
```

**Fields** get a single-line comment above them (not end-of-line). The comment is a description; it does not repeat the field name. For fields with kubebuilder markers, the comment precedes the markers:

```go
// LocalASN is the BGP Autonomous System Number for this router.
// +kubebuilder:validation:Required
LocalASN int64 `json:"localASN"`
```

Do not write comments that explain what the code obviously does. Write them when the WHY is non-obvious — a hidden constraint, a subtle invariant (e.g., the ASN int64 requirement).

### Testing

- Framework: stdlib `testing` only — no testify, no ginkgo.
- Style: table-driven tests with `t.Run` for multiple cases; single function for unique behaviors.
- Test function names: `TestTypeName_Behavior` or `TestTypeName` (e.g., `TestBGPPeerDeepCopy`, `TestUpdatePeerConditions_Established`).
- Helper constructors (e.g., `newTestPeer()`, `newTestRouter()`) are unexported and defined at the top of the test file.
- Helper lookup functions (e.g., `findCondition`) are pure functions — no `t.Helper()` required.
- Tests are in the same package as the production code (white-box, not `_test` suffix package).
- Unit tests cover: DeepCopy correctness, JSON round-trip, field name verification, and boundary values.
- E2E tests use [Chainsaw](https://kyverno.github.io/chainsaw/) against a live kind cluster. See `test/e2e/`.

### Dependencies and Imports

- `go.sum` is committed; no vendor directory.
- Imports are grouped in two blocks (gofmt standard): stdlib then external. No blank line between the groups unless there is an internal package.
- Aliases follow these conventions:
  - `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` — always aliased
  - `"k8s.io/apimachinery/pkg/api/meta"` — used unaliased
- No internal packages — all types are exported API surface.

Adding new dependencies: this repo is API-only. New dependencies must have a clear API type requirement (Kubernetes ecosystem only). Do not add runtime or application dependencies.

### Git and Version Control

**Commit messages** follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): short imperative description

Optional body explaining the why.
```

Types in use: `feat`, `fix`, `refactor`, `chore`, `ci`, `style`.
Scopes in use: `api`, `bgp`, `vpc`, `e2e`, `docs`, `fmt`, `proto`, `controller`.

**Branch naming**: `type/kebab-case-description`

```
feat/bgp-peer-status-consolidated
fix/asn-int64-schema
refactor/bgp-api-v3
```

**Merge strategy**: merge commits (PRs land as merge commits; no squash or rebase).

---

## Go Conventions

### Module and Package Layout

- Module path: `go.miloapis.com/cosmos`
- Go version: 1.26
- Package name equals the directory's last path segment (`v1alpha1`).
- One package per API group version. All types for `bgp.miloapis.com/v1alpha1` live in `api/bgp/v1alpha1/`; one `_types.go` file per CRD.
- No `internal/`, `pkg/`, or `cmd/` packages — this is an API-only module.

### Type Naming

| Construct               | Rule                                          | Example                                        |
|-------------------------|-----------------------------------------------|------------------------------------------------|
| CRD resource            | `PascalCase` prefixed with group abbreviation | `BGPRouter`, `BGPPeer`, `VPC`                  |
| Spec                    | `{Resource}Spec`                              | `BGPRouterSpec`                                |
| Status                  | `{Resource}Status`                            | `BGPRouterStatus`                              |
| List                    | `{Resource}List`                              | `BGPRouterList`                                |
| Enum type               | `{Resource}{Field}` or descriptive noun       | `BGPRouterPhase`, `BGPPeerState`               |
| Enum constant           | `{TypeName}{Value}`                           | `BGPRouterPhasePending`, `RouterRoleFabric`    |
| Condition type constant | `ConditionType{Name}` (`string`)              | `ConditionTypeReady`, `ConditionTypeAccepted`  |
| Idle reason constant    | `IdleReason{Name}` (`string`)                 | `IdleReasonBackOff`                            |
| Shared struct           | Descriptive PascalCase                        | `RouterTarget`, `AddressFamily`, `LocalSecretRef` |
| Reference struct        | `{Target}Ref`                                 | `RouterRef`, `TargetRef`                       |
| Selector struct         | `{Target}Selector`                            | `RouterSelector`                               |

Condition type and reason constants are defined as untyped `string` constants (not a named type) in the same file as the resource they belong to.

### kubebuilder Markers

Every CRD root type requires these markers directly above the type declaration, in this order:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=<abbrev>
// +kubebuilder:printcolumn:name="...",type="...",JSONPath="..."
// ... additional printcolumns
type BGPFoo struct { ... }
```

Field-level validation markers appear directly above the field comment:

```go
// LocalASN is the BGP Autonomous System Number for this router.
// +kubebuilder:validation:Required
// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:Maximum=4294967295
LocalASN int64 `json:"localASN"`
```

CEL validation uses `XValidation`:

```go
// +kubebuilder:validation:XValidation:rule="isIP(self)",message="address must be a valid IPv4 or IPv6 address"
```

CEL functions `isIP()` and `isCIDR()` require Kubernetes 1.28+.

For lists that are struct maps, use `+listType=map` and `+listMapKey=<key>`:

```go
// +listType=map
// +listMapKey=type
Conditions []metav1.Condition `json:"conditions,omitempty"`
```

For lists that enforce uniqueness, use `+listType=set`:

```go
// +listType=set
Prefixes []string `json:"prefixes"`
```

### Status Conditions

All status types that surface runtime state include:

1. `ObservedGeneration int64` — set to the `.metadata.generation` the status was computed from.
2. `Conditions []metav1.Condition` — with `+listType=map` and `+listMapKey=type`.

```go
type BGPFooStatus struct {
    // +optional
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // +listType=map
    // +listMapKey=type
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

Condition type constants (`ConditionTypeReady`, `ConditionTypeAccepted`) are `string` constants, not a named type. Use `meta.SetStatusCondition` from `k8s.io/apimachinery/pkg/api/meta` to update conditions (it handles deduplication by type).

### Field Types and Invariants

**ASN fields must use `int64`**, never `int32`. The maximum valid ASN (4294967295) exceeds int32's maximum (2147483647). Using int32 produces an invalid OpenAPI v3 schema.

**Router ID** is an IPv4 address expressed as a string with `// +kubebuilder:validation:Format=ipv4`. Even in IPv6-only underlays it is a logical identifier only.

**Duration fields** use `*metav1.Duration` (pointer, so they can be omitted). CEL validation rules enforce hold-timer semantics.

### JSON Serialization

- Required fields: `json:"fieldName"` (no omitempty).
- Optional fields: `json:"fieldName,omitempty"`.
- Inline embedding: `json:",inline"` (no field name, e.g., `metav1.TypeMeta`, `metav1.ObjectMeta`, `RouterTarget`).
- JSON field names are camelCase matching the Go field name (first letter lowercased).
- Pointer fields (`*T`) are used exclusively for optional fields that can be absent vs. zero-valued.

### Code Generation

Two generated artifacts — never edit directly:

| Artifact                   | Generator      | Regeneration command |
|----------------------------|----------------|----------------------|
| `zz_generated.deepcopy.go` | controller-gen | `task generate`      |
| `config/crd/*.yaml`        | controller-gen | `task manifests`     |

After changing any kubebuilder markers or adding/removing types: run both `task generate` and `task manifests`.

The copyright/license header for generated files is in `hack/boilerplate.go.txt`.

### Error Handling

This is an API-only module — no request path, no controllers. Error handling is minimal:

- Type methods that update status use the Kubernetes condition pattern (no returned `error`).
- `fmt.Errorf` is used in status message strings, not for wrapping errors.
- No panics in this codebase (init() only registers with the scheme builder).
- No logging — API types have no logging infrastructure.

### Go Testing

- Framework: stdlib `testing` (no external test framework).
- Table-driven via `t.Run` for parametric cases; dedicated function for single behaviors.
- Test constructors are named `newTest{TypeName}(...)` (unexported, top of test file).
- Pure helper functions (e.g., `findCondition`) do not call `t.Helper()` — they are not test helpers, they are utilities.
- Tests call methods under test then use direct `t.Errorf` / `t.Fatalf` with `got %v, want %v` format.
- Boundary value tests (especially ASN limits) are regression tests — they are named explicitly as such.
- Run unit tests: `GOOS=linux go test ./api/bgp/v1alpha1/... -v`

### Linting and Formatting

- Linter: `golangci-lint` v2.1.6 — run via `task lint`.
- Formatter: `go fmt` runs automatically as a dependency of `task build` and `task test:unit`.
- `go vet` runs with `GOOS=linux` to catch cross-platform issues.
- No `.golangci.yml` config file — default golangci-lint settings apply.

---

## YAML Conventions

- All YAML files **must** use the `.yaml` extension. Files with `.yml` will fail the lint check (`task lint` enforces this).
- YAML is formatted with `yamlfmt` v0.21.0. Run `task lint-fix` to auto-format.
- CRD manifests in `config/crd/` are generated — do not edit manually.
- Chainsaw test files follow the Chainsaw schema in `test/e2e/tests/`.

---

## Markdown Conventions

- Markdown tables must have **aligned columns** — pad each cell with spaces so the `|` delimiters line up across all rows, including the separator row. Example:

  ```markdown
  | Construct  | Rule                           | Example         |
  |------------|--------------------------------|-----------------|
  | CRD root   | PascalCase, group prefix       | `BGPRouter`     |
  | Spec type  | `{Resource}Spec`               | `BGPRouterSpec` |
  ```

  Not:

  ```markdown
  | Construct | Rule | Example |
  |-----------|------|---------|
  | CRD root | PascalCase, group prefix | `BGPRouter` |
  ```

- This applies to all `.md` files in the repository: docs, CONVENTIONS.md, ARCHITECTURE.md, CLAUDE.md, and inline docs.

---

## For Claude

- **The single most important rule**: This is API-only Go. No controllers, no runtime, no logging, no error returns from type methods. Every new file goes in `api/<group>/v1alpha1/` and follows the `_types.go` naming pattern.
- **ASN fields are always `int64`**. Never `int32`. Max ASN (4294967295) overflows int32. This is a hard invariant — the lint tests would catch it but the schema would silently break.
- **Never edit `zz_generated.deepcopy.go` or `config/crd/*.yaml`**. Run `task generate` / `task manifests` instead.
- **YAML files use `.yaml` extension only** — `.yml` fails CI.
- New CRD resource → one `{resource}_types.go` file in the appropriate `api/<group>/v1alpha1/` package. Shared types used across resources in the same package go in `shared_types.go`.
- Condition type constants are untyped `string` constants (not a named type), defined in the same file as the resource they describe.
- Status types always include `ObservedGeneration int64` and `Conditions []metav1.Condition` with `+listType=map +listMapKey=type` markers.
- Tests use stdlib `testing` only — no testify. Table-driven with `t.Run`, same-package (not `_test` suffix).
- Commit messages follow Conventional Commits: `type(scope): message`. Scopes are `api`, `bgp`, `vpc`, `e2e`, `docs`.
- **Three most common mistakes to avoid**:
  1. Using `int32` for ASN fields — always use `int64`.
  2. Adding omitempty to required fields or omitting it from optional fields.
  3. Editing generated files directly instead of updating markers and re-running `task generate` + `task manifests`.
- Comply with `go fmt` (auto-run by task), `go vet` (`GOOS=linux`), and `golangci-lint` defaults.
