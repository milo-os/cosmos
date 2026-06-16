# Cosmos

[![CI](https://github.com/milo-os/cosmos/actions/workflows/ci.yaml/badge.svg)](https://github.com/milo-os/cosmos/actions/workflows/ci.yaml)

Cosmos is a BGP control plane for Kubernetes. You define instances, sessions,
advertisements, and VPCs as Kubernetes resources; the controller reconciles
them into remote BGP agents on each node and programs learned routes into the
kernel via [netlink][netlink].

**API groups:**
`bgp.miloapis.com/v1alpha1` · `providers.bgp.miloapis.com/v1alpha1` · `vpc.miloapis.com/v1alpha1`

---

## How it works

Cosmos runs as a DaemonSet in the `bgp-system` namespace. On each node, one or
more `BGPProvider` resources represent remote BGP agent processes that cosmos
connects to via gRPC. Each provider exposes the `BGPProviderService` proto
interface; cosmos has no built-in knowledge of what runs behind it.

BGP CRDs (`BGPInstance`, `BGPPeer`, `BGPAdvertisement`, `BGPRoutePolicy`) select
providers by label and marshal configuration calls through the provider interface.
The remote agents implement whatever BGP daemon logic they need — cosmos only
cares that they satisfy the gRPC contract.

At startup the controller auto-bootstraps a `BGPProvider` resource for each
local agent endpoint. These providers are the unit of targeting for all other
resources.

Cosmos operates in a multi-cluster model:

1. A **management cluster** holds `BGPSession` and `BGPExternalPeer` resources.
   The management cluster cosmos resolves peer addresses and writes fully
   self-contained session specs.
2. [Karmada][karmada] propagates `BGPSession` resources to **POP and infra
   clusters**.
3. On each member cluster, the `SessionReconciler` generates `BGPPeer` resources
   from the propagated sessions, and the `PeerReconciler` configures the agents
   over [gRPC][grpc].

The controller has no built-in knowledge of datacenter topology — all topology
is expressed through CRDs and Karmada propagation policies.

---

## Key features

- **Provider abstraction.** `BGPProvider` resources decouple topology from agent
  implementation. Cosmos drives any agent that implements the gRPC provider interface.
- **Label-driven dispatch.** All resources select providers via `providerSelector`.
  Topology is expressed through labels, not hardcoded names.
- **Auto-bootstrapped providers.** No manual agent registration; the controller
  creates `BGPProvider` resources at startup.
- **Multi-cluster propagation.** Sessions are written once in the management
  cluster and distributed by Karmada.
- **Per-provider status.** Every BGP resource tracks reconciliation state
  independently per provider.
- **CNI-independent.** Works with any [CNI][cni] or none at all.
- **VPC primitives.** `VPC` and `VPCAttachment` CRDs model virtual networks and
  their interface bindings.

---

## Prerequisites

- Multi-cluster Kubernetes with IPv6 enabled (nodes need global-scope IPv6 addresses)
- [Karmada][karmada] for resource propagation (management → POP/infra clusters)
- `kubectl` configured for your clusters
- Container images accessible to your clusters:
  - `ghcr.io/milo-os/cosmos:latest` — controller
  - One or more remote BGP agent images that implement the `BGPProviderService` proto

---

## Quick start

Install the CRDs and deploy the controller on a member cluster:

```bash
kubectl apply -k config/crd
kubectl apply -k config/deploy
```

After the DaemonSet pods become ready, verify that `BGPProvider` resources were
auto-bootstrapped (one per agent per node):

```bash
kubectl get bgpproviders
```

Create a `BGPInstance` to configure one BGP plane. The `providerSelector`
targets the auto-bootstrapped providers for that plane:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPInstance
metadata:
  name: underlay
spec:
  providerSelector:
    matchLabels:
      bgp.datum.net/plane: underlay
  asNumber: 65000
  addressFamilies:
    - afi: IPv6
      safi: Unicast
```

In the management cluster, create a `BGPSession` to establish peering. Sessions
are propagated to member clusters by Karmada:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPSession
metadata:
  name: node-1-to-tor-1
spec:
  fromProviderSelector:
    matchLabels:
      bgp.miloapis.com/node: node-1
      bgp.datum.net/plane: underlay
  fromInstanceRef: underlay
  toPeers:
    - address: "2001:db8:fabric::1"
      asNumber: 65000
      instanceRef: underlay
  addressFamilies:
    - afi: IPv6
      safi: Unicast
```

After Karmada propagation, verify generated peer resources and session state:

```bash
kubectl get bgppeers
```

For a complete walkthrough, see the [Getting Started guide](docs/getting-started.md).

---

## API reference

### BGP — `bgp.miloapis.com/v1alpha1`

| Resource            | Short name | Description                                                                          |
|---------------------|------------|--------------------------------------------------------------------------------------|
| `BGPInstance`       | `bgpi`     | Speaker identity: AS number, address families, timers. Targets providers by selector.|
| `BGPExternalPeer`   | `bgpep`    | Registry entry for peers outside the cosmos-managed fleet. Management cluster only.  |
| `BGPPeer`           | `bgppr`    | Per-provider peer configuration. Generated by `SessionReconciler`. Never written directly.|
| `BGPSession`        | `bgps`     | Bilateral session intent. Written in management cluster; propagated via Karmada.     |
| `BGPAdvertisement`  | `bgpadv`   | Infrastructure prefix advertisement (loopbacks, SRv6 locators). Not for workload routes.|
| `BGPRoutePolicy`    | `bgprp`    | Import/export route filtering applied to matched `BGPPeer` resources.                |

### Providers — `providers.bgp.miloapis.com/v1alpha1`

| Resource      | Short name | Description                                                              |
|---------------|------------|--------------------------------------------------------------------------|
| `BGPProvider` | `bgpp`     | One remote BGP agent instance. Auto-bootstrapped at startup.             |

### VPC — `vpc.miloapis.com/v1alpha1`

| Resource        | Short name | Description                                               |
|-----------------|------------|-----------------------------------------------------------|
| `VPC`           | —          | Virtual network with one or more CIDR prefixes.           |
| `VPCAttachment` | —          | Binds a VPC to a named interface with assigned addresses. |

---

## Development

Install development tools first:

```bash
task tools
```

| Command           | Description                                                          |
|-------------------|----------------------------------------------------------------------|
| `task build`      | Compile all packages                                                 |
| `task test`       | Run unit tests                                                       |
| `task lint`       | Run golangci-lint                                                    |
| `task lint-fix`   | Run golangci-lint and apply auto-fixes                               |
| `task vet`        | Run go vet                                                           |
| `task fmt`        | Run go fmt                                                           |
| `task generate`   | Regenerate deepcopy methods                                          |
| `task manifests`  | Regenerate CRD manifests from API types                              |
| `task image`      | Build the container image                                            |
| `task image-push` | Build and push the container image                                   |
| `task test:unit`  | Run unit tests                                                       |
| `task test:e2e`   | Create a kind cluster, deploy, run the full E2E suite, and tear down |
| `task ci`         | Run the full CI pipeline locally (build, vet, unit tests, and e2e)  |
| `task clean`      | Remove build artifacts and temporary files                           |

E2E tests use [Chainsaw][chainsaw] and run against a [kind][kind] cluster. The
full suite also runs in CI on every pull request.

Build the container image:

```bash
docker build -f build/Dockerfile -t ghcr.io/milo-os/cosmos:dev .
```

---

## Documentation

- [Getting started](docs/getting-started.md) — deploy and establish your first BGP sessions
- [API reference](docs/api/README.md) — full CRD field definitions, conditions, and operational contracts for all API groups
- [Service design](docs/design/) — original design proposal (archived; superseded by current API)
- [Enhancements](docs/enhancements/) — design proposals

### Examples

- [BGPProvider (auto-bootstrapped)](docs/examples/pop-bgpprovider-auto.yaml)
- [BGPInstance — underlay](docs/examples/pop-bgpinstance-underlay.yaml)
- [BGPInstance — overlay](docs/examples/pop-bgpinstance-overlay.yaml)
- [BGPInstance — infra route reflector](docs/examples/infra-bgpinstance-rr.yaml)
- [BGPExternalPeer — ToR switch](docs/examples/mgmt-bgpexternalpeer-tor.yaml)
- [BGPSession — underlay to ToR](docs/examples/mgmt-bgpsession-underlay-tor.yaml)
- [BGPSession — overlay to route reflectors](docs/examples/mgmt-bgpsession-overlay-rrs.yaml)
- [BGPSession — RR client (infra)](docs/examples/infra-bgpsession-rr-client.yaml)
- [Rejected configurations](docs/examples/rejected-configurations.yaml)

---

## License

[Apache 2.0](LICENSE)

<!-- References -->
[bgp]: https://datatracker.ietf.org/doc/html/rfc4271
[grpc]: https://grpc.io/
[karmada]: https://karmada.io/
[netlink]: https://man7.org/linux/man-pages/man7/netlink.7.html
[cni]: https://www.cni.dev/
[rr]: https://datatracker.ietf.org/doc/html/rfc4456
[chainsaw]: https://kyverno.github.io/chainsaw/
[kind]: https://kind.sigs.k8s.io/
