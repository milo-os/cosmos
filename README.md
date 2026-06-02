# Cosmos

[![CI](https://github.com/milo-os/cosmos/actions/workflows/ci.yaml/badge.svg)](https://github.com/milo-os/cosmos/actions/workflows/ci.yaml)

Cosmos is a BGP control plane for Kubernetes. You define speakers, sessions,
policies, and VPCs as Kubernetes resources; the controller reconciles them into
a running [GoBGP][gobgp] instance on each node and programs learned routes into
the kernel.

**API groups:** `bgp.miloapis.com/v1alpha1` · `vpc.miloapis.com/v1alpha1`

---

## How it works

Cosmos runs as a DaemonSet. Each pod has two runtime containers:

- **`bgp`** — the controller; watches CRDs and configures GoBGP over [gRPC][grpc]
- **`gobgpd`** — the GoBGP daemon; handles the BGP wire protocol

An init container discovers the node's global IPv6 address and writes the
initial GoBGP configuration before the main containers start. The controller
programs kernel routes from BGP [RIB][rib] events via [netlink][netlink].

The controller has no built-in knowledge of nodes, clusters, or datacenter
layout — all topology is expressed through CRDs.

---

## Key features

- **Topology-agnostic.** No built-in node or cluster model; topology lives entirely in CRDs.
- **CNI-independent.** Works with any [CNI][cni] or none at all.
- **Declarative session management.** `BGPPeeringPolicy` creates sessions through label selectors; full-mesh and [route-reflector][rr] topologies both supported.
- **Producer/consumer model.** Any system — node operators, automation pipelines, humans — can create BGP CRDs; the controller reconciles them uniformly.
- **VPC primitives.** `VPC` and `VPCAttachment` CRDs model virtual networks and their interface bindings.

---

## Prerequisites

- Kubernetes cluster with IPv6 enabled (nodes need global-scope IPv6 addresses)
- `kubectl` configured for your cluster
- Container images accessible to your cluster:
  - `ghcr.io/milo-os/cosmos:latest` — controller + init container
  - A GoBGP daemon image (`gobgpd:latest` by default)

---

## Quick start

Install the CRDs and deploy the controller:

```bash
kubectl apply -k config/crd
kubectl apply -k config/deploy
```

Create a `BGPConfiguration` (one per cluster):

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPConfiguration
metadata:
  name: default
spec:
  asNumber: 65001
  listenPort: 1790
  routerIDSource: NodeIP
```

Create `BGPEndpoint` resources for your nodes and wire them together with a
peering policy:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPEndpoint
metadata:
  name: node-worker-01
  labels:
    bgp.miloapis.com/role: node
spec:
  address: "2001:db8:1::1"
  asNumber: 65001
---
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeeringPolicy
metadata:
  name: mesh
spec:
  selector:
    matchLabels:
      bgp.miloapis.com/role: node
  mode: mesh
```

Check session state:

```bash
kubectl get bgpsess
```

For a complete walkthrough, see the [Getting Started guide](docs/getting-started.md).

---

## API reference

### BGP — `bgp.miloapis.com/v1alpha1`

| Resource           | Short name   | Description                                                          |
|--------------------|--------------|----------------------------------------------------------------------|
| `BGPConfiguration` | `bgpconfig`  | Speaker identity: AS number, listen port, router ID. One per cluster.|
| `BGPEndpoint`      | `bgpep`      | BGP speaker address and AS number. One per node.                     |
| `BGPSession`       | `bgpsess`    | Peering relationship between two endpoints.                          |
| `BGPPeeringPolicy` | `bgppp`      | Automates session creation via label selectors.                      |
| `BGPAdvertisement` | `bgpadvert`  | Prefix advertisement with optional communities and LOCAL_PREF.       |
| `BGPRoutePolicy`   | `bgprp`      | Import/export filtering with prefix matching.                        |

### VPC — `vpc.miloapis.com/v1alpha1`

| Resource        | Description                                                   |
|-----------------|---------------------------------------------------------------|
| `VPC`           | Virtual network with one or more CIDR prefixes.               |
| `VPCAttachment` | Binds a VPC to a named interface with assigned addresses.     |

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
| `task vet`        | Run go vet                                                           |
| `task generate`   | Regenerate deepcopy methods                                          |
| `task manifests`  | Regenerate CRD manifests from API types                              |
| `task test-e2e`   | Create a kind cluster, deploy, run the full E2E suite, and tear down |

E2E tests use [Chainsaw][chainsaw] and run against a [kind][kind] cluster. The
full suite also runs in CI on every pull request.

Build the container image:

```bash
docker build -f build/Dockerfile -t ghcr.io/milo-os/cosmos:dev .
```

---

## Documentation

- [Getting started](docs/getting-started.md) — deploy and establish your first BGP session
- [Service design](docs/design/) — architecture, controller internals, and design decisions
- [Enhancements](docs/enhancements/) — accepted design proposals

### Examples

- [Single-cluster full mesh](docs/examples/single-cluster-mesh.yaml)
- [Route-reflector topology](docs/examples/route-reflector.yaml)
- [eBGP peering](docs/examples/ebgp-peering.yaml)
- [Prefix advertisement with communities](docs/examples/prefix-advertisement.yaml)
- [Route filtering](docs/examples/route-filtering.yaml)

---

## License

[Apache 2.0](LICENSE)

<!-- References -->
[bgp]: https://datatracker.ietf.org/doc/html/rfc4271
[gobgp]: https://github.com/osrg/gobgp
[grpc]: https://grpc.io/
[rib]: https://en.wikipedia.org/wiki/Routing_table
[netlink]: https://man7.org/linux/man-pages/man7/netlink.7.html
[cni]: https://www.cni.dev/
[rr]: https://datatracker.ietf.org/doc/html/rfc4456
[chainsaw]: https://kyverno.github.io/chainsaw/
[kind]: https://kind.sigs.k8s.io/
