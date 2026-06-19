---
status: implementable
stage: alpha
---

# BGP API Reference

**API Group:** `bgp.miloapis.com/v1alpha1`
**Stability:** Alpha — fields and defaults may change without deprecation notice

---

## 1. Overview

Cosmos provides Kubernetes APIs for expressing BGP routing intent. It is an
API project: it defines resources, relationships, validation, and status
contracts. Cosmos does not define how routing intent is realized.

The fleet uses two BGP planes per node:

| Plane | Purpose |
|-------|---------|
| **Underlay** | IPv6 unicast fabric routing between nodes and top-of-rack switches |
| **Overlay** | L2VPN EVPN distribution for tenant workloads |

Each plane is represented by a separate `BGPRouter` resource. `BGPPeer`,
`BGPAdvertisement`, and `BGPPolicy` resources target a router by direct
reference (`routerRef`) or label selector (`routerSelector`).

### API Group

| Group | Purpose |
|-------|---------|
| `bgp.miloapis.com/v1alpha1` | BGP routing context, sessions, advertisements, and policies |

All resources are **Namespaced**.

---

## 2. CRD Reference

| Kind | Short Name | Targeting |
|------|------------|-----------|
| [BGPRouter](#bgprouter) | `bgpr` | — |
| [BGPPeer](#bgppeer) | `bgppr` | `routerRef` XOR `routerSelector` |
| [BGPAdvertisement](#bgpadvertisement) | `bgpadv` | `routerRef` only |
| [BGPPolicy](#bgppolicy) | `bgpp` | `routerRef` XOR `routerSelector` |

---

## 3. Common Types

### AddressFamily

Address families are expressed as an AFI/SAFI struct. Invalid combinations
are rejected at the CRD validation layer.

```yaml
addressFamilies:
  - afi: ipv6
    safi: unicast
```

| AFI     | SAFI      | Use                             |
|---------|-----------|---------------------------------|
| `ipv4`  | `unicast` | IPv4 unicast routing            |
| `ipv6`  | `unicast` | IPv6 unicast routing (underlay) |
| `l2vpn` | `evpn`    | EVPN overlay routing            |

### RouterTarget

Resources that can target one or more routers embed `RouterTarget`. Exactly
one of `routerRef` or `routerSelector` must be set — both or neither is
rejected by CEL validation.

```yaml
# Direct reference (single router)
routerRef:
  name: node-1-underlay

# Label selector (multiple routers)
routerSelector:
  matchLabels:
    bgp.miloapis.com/role: fabric
```

---

### BGPRouter

`BGPRouter` defines a logical BGP routing context. It identifies the execution
target, local AS number, router ID, functional roles, and address families. It
is the primary ownership boundary for `BGPPeer`, `BGPAdvertisement`, and
`BGPPolicy` resources.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `targetRef` | `TargetRef` | Yes | Identifies the execution target. |
| `targetRef.kind` | `string` | Yes | Target resource kind. Supported: `Node`. |
| `targetRef.name` | `string` | Yes | Name of the target resource. |
| `roles` | `[]RouterRole` | Yes | Functional roles. Minimum 1. |
| `localASN` | `uint32` | Yes | Local AS number. Range: 1–4294967295. |
| `routerID` | `string` | Yes | Router ID in IPv4 dotted-decimal notation. |
| `addressFamilies` | `[]AddressFamily` | Yes | Address families this router activates. Minimum 1. |

**RouterRole** (string enum)

| Value | Meaning |
|-------|---------|
| `fabric` | Router participates in the internal fabric (underlay or overlay). |
| `tenant` | Router serves a tenant workload network. |
| `transit` | Router carries transit traffic between autonomous systems. |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | Current phase. Enum: `Pending`, `Ready`, `Failed`. |
| `observedGeneration` | `int64` | Last spec generation reflected in this status. |
| `roles` | `[]RouterRole` | Active roles as observed by the implementation. |
| `peers` | `BGPRouterPeerSummary` | Peer session counts. |
| `conditions` | `[]metav1.Condition` | Top-level conditions. |

**BGPRouterPeerSummary**

| Field | Type | Description |
|-------|------|-------------|
| `total` | `int32` | Total number of configured peers. |
| `established` | `int32` | Count of peers currently in Established state. |

**BGPRouter Conditions**

| Type | Required | Meaning when True |
|------|----------|-------------------|
| `Ready` | Required | Router is fully configured and the runtime is accepting routing intent. |
| `RuntimeAvailable` | Required | The routing runtime is reachable and accepting configuration. |
| `ConfigApplied` | Required | Current spec has been translated and applied to the runtime. |
| `Degraded` | Required | One or more configured peers has not reached Established state. |
| `PeersEstablished` | Optional | All configured peers have reached Established state. |

#### Example

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

### BGPPeer

`BGPPeer` defines a BGP session to a remote peer. It binds to one or more
`BGPRouter` instances via `routerRef` or `routerSelector`.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `routerRef` | `RouterRef` | Conditional | Direct reference to a single BGPRouter. Mutually exclusive with `routerSelector`. |
| `routerSelector` | `RouterSelector` | Conditional | Label selector for BGPRouter resources. Mutually exclusive with `routerRef`. |
| `peerASN` | `uint32` | Yes | Remote AS number. Range: 1–4294967295. |
| `address` | `string` | Yes | Remote peer's IPv4 or IPv6 address. |
| `description` | `string` | No | Human-readable label for this peer (e.g., `"spine-1"`). |
| `authSecretRef` | `LocalSecretRef` | No | References a Secret containing the MD5 TCP authentication password under key `"password"`. |
| `addressFamilies` | `[]AddressFamily` | Yes | Address families negotiated on this session. Minimum 1. |
| `holdTime` | `Duration` | No | BGP hold timer. Must be 0 (disabled) or ≥ 3s. Default: `90s`. |
| `keepaliveTime` | `Duration` | No | BGP keepalive interval. Must be ≤ holdTime / 3. Default: `30s`. |

**LocalSecretRef**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the Secret in the same namespace. |

> Timer fields use Go duration strings (e.g., `"90s"`, `"1m30s"`). Both forms
> are valid; implementations must accept either.

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `observedGeneration` | `int64` | Last spec generation reflected in this status. |
| `sessionState` | `string` | Current BGP FSM state. |
| `lastEstablishedTime` | `Time` | Timestamp of the most recent Established transition. |
| `conditions` | `[]metav1.Condition` | Top-level conditions. |

**Session State** (string enum)

`Idle` → `Connect` → `Active` → `OpenSent` → `OpenConfirm` → `Established`

**BGPPeer Conditions**

| Type | Required | Meaning when True |
|------|----------|-------------------|
| `Ready` | Required | Session is Established and address families have been negotiated. |
| `Accepted` | Required | Peer config has been accepted by the runtime. |
| `SessionIdle` | Required | BGP FSM is in Idle state. |
| `SessionConnect` | Required | BGP FSM is in Connect state. |
| `SessionActive` | Required | BGP FSM is in Active state. |
| `SessionOpenSent` | Required | BGP FSM is in OpenSent state. |
| `SessionOpenConfirm` | Required | BGP FSM is in OpenConfirm state. |
| `SessionEstablished` | Required | BGP FSM is in Established state. |

The session FSM state conditions (`SessionIdle` through `SessionEstablished`)
are mutually exclusive — exactly one must be `True` at any time.

#### Example

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeer
metadata:
  name: node-1-to-tor-1
  namespace: default
spec:
  routerRef:
    name: node-1-underlay
  peerASN: 65000
  address: "2001:db8:fabric::1"
  description: "tor-1"
  addressFamilies:
    - afi: ipv6
      safi: unicast
  holdTime: 90s
  keepaliveTime: 30s
```

---

### BGPAdvertisement

`BGPAdvertisement` defines routing information to advertise from a single
`BGPRouter`. Prefixes are specified inline. Only `routerRef` is supported —
selector fan-out is intentionally omitted to avoid ambiguous prefix attribution.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `routerRef` | `RouterRef` | Yes | Direct reference to a single BGPRouter. |
| `addressFamily` | `AddressFamily` | Yes | AFI/SAFI for this advertisement. |
| `prefixes` | `[]string` | Yes | CIDR prefixes to advertise. Minimum 1. |
| `communities` | `[]string` | No | BGP communities to attach to advertised prefixes. |
| `localPreference` | `*uint32` | No | BGP LOCAL_PREF attribute. Only meaningful for iBGP sessions. |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `observedGeneration` | `int64` | Last spec generation reflected in this status. |
| `advertisedPrefixes` | `int32` | Count of prefixes currently being originated. |
| `conditions` | `[]metav1.Condition` | Top-level conditions. |

**BGPAdvertisement Conditions**

| Type | Required | Meaning when True |
|------|----------|-------------------|
| `Ready` | Required | Advertisement is active and prefixes are being originated. |
| `Accepted` | Required | Advertisement config accepted by the runtime. |
| `Advertised` | Required | Prefixes are confirmed as advertised to at least one peer. |

#### Example

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPAdvertisement
metadata:
  name: node-1-loopback
  namespace: default
spec:
  routerRef:
    name: node-1-underlay
  addressFamily:
    afi: ipv6
    safi: unicast
  prefixes:
    - "2001:db8:loopback::1/128"
  localPreference: 100
```

---

### BGPPolicy

`BGPPolicy` defines composable, ordered routing policy statements applied
to a BGPRouter in a specific direction (import or export). It binds to one or
more `BGPRouter` instances via `routerRef` or `routerSelector`.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `routerRef` | `RouterRef` | Conditional | Direct reference to a single BGPRouter. Mutually exclusive with `routerSelector`. |
| `routerSelector` | `RouterSelector` | Conditional | Label selector for BGPRouter resources. Mutually exclusive with `routerRef`. |
| `direction` | `string` | Yes | Policy direction. Enum: `import`, `export`. |
| `terms` | `[]BGPPolicyTerm` | Yes | Ordered list of policy statements. Minimum 1. Evaluated ascending by `sequence`. |

**BGPPolicyTerm**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sequence` | `int32` | Yes | Evaluation order. Range: 1–65535. Must be unique within the policy. |
| `match` | `BGPPolicyMatch` | Yes | Conditions under which this term fires. |
| `action` | `string` | Yes | Disposition on match. Enum: `permit`, `deny`. |
| `set` | `PolicySetActions` | No | Mutations applied on `permit`. Must not be set on `deny` terms. |

**BGPPolicyMatch**

| Field | Type | Description |
|-------|------|-------------|
| `any` | `bool` | When true, matches all routes. Mutually exclusive with other match fields. |
| `addressFamilies` | `[]AddressFamily` | Constrains match to specific AFI/SAFI combinations. If empty, all address families are matched. |

**PolicySetActions**

| Field | Type | Description |
|-------|------|-------------|
| `communities` | `CommunitySet` | Community add/remove operations. |
| `localPreference` | `*uint32` | Sets LOCAL_PREF. Only meaningful on import (iBGP) or export to iBGP peers. |

**CommunitySet**

| Field | Type | Description |
|-------|------|-------------|
| `add` | `[]string` | Communities to attach (e.g., `"65000:100"`). |
| `remove` | `[]string` | Communities to strip. |

#### Validation

- Term `sequence` numbers must be unique within a policy (CEL enforced).
- `set` actions must not be specified on `deny` terms (CEL enforced).

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `observedGeneration` | `int64` | Last spec generation reflected in this status. |
| `conditions` | `[]metav1.Condition` | Top-level conditions. |

**BGPPolicy Conditions**

| Type | Required | Meaning when True |
|------|----------|-------------------|
| `Ready` | Required | Policy is active and applied to at least one router. |
| `Accepted` | Required | Policy config accepted by the runtime. |
| `PolicyApplied` | Required | Policy terms are confirmed applied to the target routers. |

#### Examples

```yaml
# Export policy — tag fabric routes and deny everything else
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPolicy
metadata:
  name: fabric-export-policy
  namespace: default
spec:
  routerSelector:
    matchLabels:
      bgp.miloapis.com/role: fabric
  direction: export
  terms:
    - sequence: 10
      match:
        addressFamilies:
          - afi: ipv6
            safi: unicast
      action: permit
      set:
        communities:
          add: ["65001:100"]
          remove: ["65001:200"]
        localPreference: 150
    - sequence: 20
      match:
        any: true
      action: deny
```

```yaml
# Import policy — accept EVPN routes and tag them
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPolicy
metadata:
  name: evpn-import-filter
  namespace: default
spec:
  routerSelector:
    matchLabels:
      bgp.miloapis.com/role: tenant
  direction: import
  terms:
    - sequence: 10
      match:
        addressFamilies:
          - afi: l2vpn
            safi: evpn
      action: permit
      set:
        communities:
          add: ["65001:200"]
    - sequence: 20
      match:
        any: true
      action: deny
```

---

## 4. Design Principles

**Intent over implementation.** The API describes desired routing state. It
must not expose implementation details (FRR, GoBGP, Bird, JunOS, Linux VRFs,
interfaces, routing tables). Implementations consume the API and realize intent
using whatever runtime fits their environment.

**Ownership model.** `BGPRouter` is the primary ownership boundary. Dependent
resources associate via `routerRef` (single router) or `routerSelector`
(multiple routers by label). Exactly one targeting mechanism must be set on
any resource that supports both.

**BGPAdvertisement is router-scoped.** Advertisements always target a single
router via `routerRef`. Selector fan-out is intentionally not supported on
advertisements to avoid ambiguous prefix attribution across multiple routers.

---

## 5. Common Verification Commands

```bash
# List all BGPRouters
kubectl get bgprouters

# List all BGPPeers and their session state
kubectl get bgppeers

# Watch BGPPeer state changes
kubectl get bgppeers -w

# Describe a peer to see conditions
kubectl describe bgppeer <name>

# List advertisements
kubectl get bgpadvertisements

# List route policies
kubectl get bgppolicies
```
