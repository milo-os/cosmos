# Cosmos

[![CI](https://github.com/milo-os/cosmos/actions/workflows/ci.yaml/badge.svg)](https://github.com/milo-os/cosmos/actions/workflows/ci.yaml)

Cosmos is a Kubernetes API project for BGP routing and virtual networking
intent. It defines CRDs, validation, and status contracts — it does not ship a
controller. Implementations consume these APIs and realize intent using whatever
runtime fits their environment.

**API groups:**
`bgp.miloapis.com/v1alpha1` · `vpc.miloapis.com/v1alpha1`

---

## BGP API

The BGP API models two routing planes per node:

| Plane | Purpose |
|-------|---------|
| **Underlay** | IPv6 unicast fabric routing between nodes and top-of-rack switches |
| **Overlay** | L2VPN EVPN distribution for tenant workloads |

`BGPRouter` is the primary ownership boundary — one resource per plane per
node. All other BGP resources bind to routers via `routerRef` (single router)
or `routerSelector` (multiple routers by label).

| Resource           | Short name | Description                                                              |
|--------------------|------------|--------------------------------------------------------------------------|
| `BGPRouter`        | `bgpr`     | BGP routing context: AS number, router ID, address families, roles.      |
| `BGPPeer`          | `bgppr`    | BGP session to a remote peer. `routerRef` XOR `routerSelector`.          |
| `BGPAdvertisement` | `bgpadv`   | Prefix advertisement. `routerRef` only — single-router scope.            |
| `BGPPolicy`        | `bgpp`     | Import/export route filtering with ordered terms. `routerRef` XOR `routerSelector`. |
| `BGPVRFInstance`   | `bgpvrf`   | L2VPN EVPN VRF: route distinguisher, import/export route targets.        |

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPRouter
metadata:
  name: node-1-underlay
  namespace: default
  labels:
    bgp.miloapis.com/role: fabric
spec:
  targetRef:
    kind: Node
    name: node-1
  roles:
    - fabric
  localASN: 65000
  routerID: "10.0.0.1"
  addressFamilies:
    - afi: ipv6
      safi: unicast
```

---

## VPC API

The VPC API models virtual tenant networks and their interface bindings.

| Resource        | Description                                                              |
|-----------------|--------------------------------------------------------------------------|
| `VPC`           | Virtual network with one or more IPv4 or IPv6 CIDR prefixes.            |
| `VPCAttachment` | Binds a VPC to a named network interface with assigned addresses.        |

A `VPC` names a set of prefixes. A `VPCAttachment` connects a workload
interface to that VPC by assigning addresses and recording the binding:

```yaml
apiVersion: vpc.miloapis.com/v1alpha1
kind: VPC
metadata:
  name: tenant-a
  namespace: default
spec:
  networks:
    - "10.100.0.0/24"
    - "fd00:a::/48"
---
apiVersion: vpc.miloapis.com/v1alpha1
kind: VPCAttachment
metadata:
  name: tenant-a-node-1
  namespace: default
spec:
  vpc:
    name: tenant-a
  interface:
    name: eth0
    addresses:
      - "10.100.0.5"
      - "fd00:a::5"
```

---

## Quick start

Install the CRDs:

```bash
kubectl apply -k config/crd
```

For a complete walkthrough, see the [Getting Started guide](docs/getting-started.md).

---

## Requirements

- Kubernetes 1.28+ — CEL validation functions `isIP()` and `isCIDR()` used by this API are only available from Kubernetes 1.28 onwards.

---

## Development

Install development tools first:

```bash
task tools
```

| Command          | Description                                                           |
|------------------|-----------------------------------------------------------------------|
| `task build`     | Compile all packages                                                  |
| `task test`      | Run unit tests then e2e tests                                         |
| `task lint`      | Run golangci-lint and yamlfmt                                         |
| `task vet`       | Run go vet                                                            |
| `task fmt`       | Run go fmt                                                            |
| `task generate`  | Regenerate deepcopy methods                                           |
| `task manifests` | Regenerate CRD manifests from API types                               |
| `task test:unit` | Run unit tests                                                        |
| `task test:e2e`  | Create a kind cluster, deploy CRDs, run Chainsaw tests, and tear down |
| `task ci`        | Run the full CI pipeline locally (build, lint, unit tests, and e2e)  |
| `task clean`     | Remove build artifacts and temporary files                            |

E2E tests use [Chainsaw][chainsaw] and run against a [kind][kind] cluster. The
full suite also runs in CI on every pull request.

---

## Documentation

- [Getting started](docs/getting-started.md) — install CRDs and create your first resources
- [BGP API reference](docs/api/bgp.md) — full CRD field definitions, conditions, and validation rules
- [Design](docs/design/) — original design documentation (archived)
- [Enhancements](docs/enhancements/) — design proposals

### Examples

- [Rejected configurations](docs/examples/rejected-configurations.yaml)

---

## License

[Apache 2.0](LICENSE)

<!-- References -->
[chainsaw]: https://kyverno.github.io/chainsaw/
[kind]: https://kind.sigs.k8s.io/
