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

| Plane        | Purpose                                                            |
|--------------|--------------------------------------------------------------------|
| **Underlay** | IPv6 unicast fabric routing between nodes and top-of-rack switches |
| **Overlay**  | L2VPN EVPN distribution for tenant workloads                       |

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

| Field             | Type              | Required | Description                                                |
|-------------------|-------------------|----------|------------------------------------------------------------|
| `targetRef`       | `TargetRef`       | Yes      | Identifies the execution target.                           |
| `targetRef.kind`  | `string`          | Yes      | Target resource kind. Supported: `Node`.                   |
| `targetRef.name`  | `string`          | Yes      | Name of the target resource.                               |
| `roles`           | `[]RouterRole`    | Yes      | Functional roles. Minimum 1.                               |
| `localASN`        | `uint32`          | Yes      | Local AS number. Range: 1–4294967295.                      |
| `routerID`        | `string`          | Yes      | Router ID in IPv4 dotted-decimal notation.                 |
| `addressFamilies` | `[]AddressFamily` | Yes      | Address families this router activates. Minimum 1.         |

**RouterRole** (string enum)

| Value     | Meaning                                                           |
|-----------|-------------------------------------------------------------------|
| `fabric`  | Router participates in the internal fabric (underlay or overlay). |
| `tenant`  | Router serves a tenant workload network.                          |
| `transit` | Router carries transit traffic between autonomous systems.        |

#### Status

| Field                | Type                   | Description                                        |
|----------------------|------------------------|----------------------------------------------------|
| `phase`              | `string`               | Current phase. Enum: `Pending`, `Ready`, `Failed`. |
| `observedGeneration` | `int64`                | Last spec generation reflected in this status.     |
| `roles`              | `[]RouterRole`         | Active roles as observed by the implementation.    |
| `peers`              | `BGPRouterPeerSummary` | Peer session counts.                               |
| `conditions`         | `[]metav1.Condition`   | Top-level conditions.                              |

**BGPRouterPeerSummary**

| Field         | Type    | Description                                    |
|---------------|---------|------------------------------------------------|
| `total`       | `int32` | Total number of configured peers.              |
| `established` | `int32` | Count of peers currently in Established state. |

**BGPRouter Conditions**

| Type               | Required | Meaning when True                                                       |
|--------------------|----------|-------------------------------------------------------------------------|
| `Ready`            | Required | Router is fully configured and the runtime is accepting routing intent. |
| `RuntimeAvailable` | Required | The routing runtime is reachable and accepting configuration.           |
| `ConfigApplied`    | Required | Current spec has been translated and applied to the runtime.            |
| `Degraded`         | Required | One or more configured peers has not reached Established state.         |
| `PeersEstablished` | Optional | All configured peers have reached Established state.                    |

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

| Field             | Type              | Required | Description                                                |
|-------------------|-------------------|----------|------------------------------------------------------------|
| `routerRef`       | `RouterRef`       | Conditional | Direct reference to a single BGPRouter. Mutually exclusive with `routerSelector`. |
| `routerSelector`  | `RouterSelector`  | Conditional | Label selector for BGPRouter resources. Mutually exclusive with `routerRef`. |
| `peerASN`         | `uint32`          | Yes      | Remote AS number. Range: 1–4294967295.                     |
| `address`         | `string`          | Yes      | Remote peer's IPv4 or IPv6 address.                        |
| `description`     | `string`          | No       | Human-readable label for this peer (e.g., `"spine-1"`).    |
| `authSecretRef`   | `LocalSecretRef`  | No       | References a Secret containing the MD5 TCP authentication password under key `"password"`. |
| `addressFamilies` | `[]AddressFamily` | Yes      | Address families negotiated on this session. Minimum 1.    |
| `holdTime`        | `Duration`        | No       | BGP hold timer. Must be 0 (disabled) or ≥ 3s. Default: `90s`. |
| `keepaliveTime`   | `Duration`        | No       | BGP keepalive interval. Must be ≤ holdTime / 3. Default: `30s`. |

**LocalSecretRef**

| Field  | Type     | Required | Description                               |
|--------|----------|----------|-------------------------------------------|
| `name` | `string` | Yes      | Name of the Secret in the same namespace. |

> Timer fields use Go duration strings (e.g., `"90s"`, `"1m30s"`). Both forms
> are valid; implementations must accept either.

#### Status

| Field                 | Type                 | Description                                          |
|-----------------------|----------------------|------------------------------------------------------|
| `observedGeneration`  | `int64`              | Last spec generation reflected in this status.       |
| `sessionState`        | `string`             | Current BGP FSM state.                               |
| `lastEstablishedTime` | `Time`               | Timestamp of the most recent Established transition. |
| `conditions`          | `[]metav1.Condition` | Top-level conditions.                                |

**Session State** (string enum)

`Idle` → `Connect` → `Active` → `OpenSent` → `OpenConfirm` → `Established`

**BGPPeer Conditions**

| Type       | Required | Meaning when True                                                 |
|------------|----------|-------------------------------------------------------------------|
| `Ready`    | Required | Session is Established and address families have been negotiated. |
| `Accepted` | Required | Peer config has been accepted by the runtime.                     |

Condition type constants are defined in Go as `ConditionTypeReady` and
`ConditionTypeAccepted` in `api/bgp/v1alpha1/peer_types.go`.

The `Ready` condition reflects the BGP FSM state via its `Status` and `Reason`:

| `sessionState`  | `Ready.Status` | `Ready.Reason`         | Meaning                                       |
|-----------------|----------------|------------------------|-----------------------------------------------|
| `Established`   | `True`         | `Established`          | Session is up; all address families negotiated. |
| `OpenConfirm`   | `False`        | `OpenConfirm`          | BGP OPEN messages exchanged, holding timer running. |
| `OpenSent`      | `False`        | `OpenSent`             | OPEN message sent, awaiting OPEN from peer.   |
| `Active`        | `False`        | `Active`               | Attempting to establish (connecting or re-connecting). |
| `Connect`       | `False`        | `Connect`              | Waiting for TCP connection or initiating connection. |
| `Idle`          | `False`        | see below              | Session is idle; Reason explains why.         |

**Idle sub-reasons**

When `sessionState` is `Idle`, the `Ready.Reason` is set by the controller
based on recent events:

| Reason                | Meaning                                                    |
|-----------------------|------------------------------------------------------------|
| `BackOff`             | Exponential back-off before next connection attempt.       |
| `ConnectionRefused`   | TCP connection was actively refused by the peer.           |
| `HoldTimerExpired`    | Peer failed to send KEEPALIVE within the hold timer.       |
| `Idle`                | No recent failure; session is simply not started yet.      |

**Controller status update logic**

The reference implementation is the `BGPPeerStatus.updatePeerConditions` method
in `api/bgp/v1alpha1/peer_types.go`. Call it whenever `sessionState` changes:

```go
status.updatePeerConditions(state, peer.Generation, idleReason)
```

For the `Accepted` condition, use `SetAcceptedCondition`:

```go
status.SetAcceptedCondition(true, peer.Generation, "ConfigAccepted", msg)
```

This pattern avoids the high API server churn of maintaining 6 mutually exclusive
conditions that flip every few seconds during session establishment. The
`sessionState` string field carries the exact FSM state for detailed inspection;
the `Ready` condition provides the high-level signal that consumers (e.g. HPA,
gates, dashboards) need.

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

| Field             | Type            | Required | Description                                                |
|-------------------|-----------------|----------|------------------------------------------------------------|
| `routerRef`       | `RouterRef`     | Yes      | Direct reference to a single BGPRouter.                    |
| `addressFamily`   | `AddressFamily` | Yes      | AFI/SAFI for this advertisement.                           |
| `prefixes`        | `[]string`      | Yes      | CIDR prefixes to advertise. Minimum 1.                     |
| `communities`     | `[]string`      | No       | BGP communities to attach to advertised prefixes.          |
| `localPreference` | `*uint32`       | No       | BGP LOCAL_PREF attribute. Only meaningful for iBGP sessions. |

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
| `any` | `bool` | When true, matches all routes. Mutually exclusive with all other match fields. |
| `addressFamilies` | `[]AddressFamily` | Constrains match to specific AFI/SAFI combinations. If empty, all address families are matched. Max 8. |
| `prefixList` | `[]string` | Match routes whose prefix matches one of the given CIDR blocks. Max 256. |
| `asPathFilter` | `ASPathFilter` | Match routes by AS path regular expression. |
| `communityMatch` | `[]string` | Match routes carrying any of the listed BGP communities. Format: `ASN:NN` or `IP:NN`. Max 32. |
| `evpnRouteType` | `[]EVPNRouteType` | Match specific EVPN route types. Only meaningful with `l2vpn/evpn`. Max 5. |
| `vni` | `*uint32` | Match routes by VNI. Range: 0–16777215. |
| `macAddress` | `*string` | Match EVPN Type-2 routes by MAC address. Format: `aa:bb:cc:dd:ee:ff`. |
| `ipPrefix` | `*string` | Match routes by exact IP prefix (CIDR notation). |
| `localPreference` | `*uint32` | Match routes by LOCAL_PREF value. Range: 0–4294967295. |
| `med` | `*uint32` | Match routes by MED value. Range: 0–4294967295. |

**ASPathFilter**

| Field | Type | Description |
|-------|------|-------------|
| `pattern` | `string` | Regular expression matched against the AS path (space-separated ASNs). Required. |
| `matchType` | `string` | How the pattern is applied. Enum: `full`, `contains`. Default: `contains`. |

**EVPNRouteType** (string enum)

| Value | RFC 7432 Type | Description |
|-------|--------------|-------------|
| `inclusiveMulticastEthernetTag` | Type 1 | BUM traffic distribution |
| `macIPAdvertisement` | Type 2 | Host MAC/IP reachability |
| `iPPrefixAdvertisement` | Type 3 | L3 IP prefix distribution |
| `stickyMACAddress` | Type 4 | Sticky MAC address (MAC mobility) |
| `iPv6PrefixAdvertisement` | Type 5 | IPv6 L3 route distribution |

**PolicySetActions**

| Field | Type | Description |
|-------|------|-------------|
| `communities` | `CommunitySet` | Standard community add/remove operations. |
| `localPreference` | `*uint32` | Sets LOCAL_PREF. Only meaningful on import (iBGP) or export to iBGP peers. Range: 0–4294967295. |
| `origin` | `*string` | Sets the BGP origin attribute. Enum: `igp`, `egp`, `incomplete`. |
| `asPath` | `AsPathSet` | Manipulates the AS path (prepend or replace). |
| `nextHop` | `NextHopSet` | Overrides the next-hop attribute. |
| `extCommunities` | `ExtendedCommunitySet` | Extended community add/remove operations. |
| `metric` | `*uint32` | Sets MED (Multi-Exit Discriminator). Range: 0–4294967295. |
| `color` | `*uint32` | Sets the SRv6 policy color for path selection. Range: 0–4294967295. |
| `srv6EndpointBehavior` | `*string` | Sets the SRv6 endpoint behavior on a route (e.g., `End`, `End.X`, `End.DT6`). |

**CommunitySet**

| Field | Type | Description |
|-------|------|-------------|
| `add` | `[]string` | Communities to attach (e.g., `"65000:100"`). Max 32. |
| `remove` | `[]string` | Communities to strip. Max 32. |

**AsPathSet**

| Field | Type | Description |
|-------|------|-------------|
| `prepend` | `*uint32` | Number of times to prepend the local ASN (or `asn`) to the AS path. Range: 1–10. Mutually exclusive with `replace`. |
| `asn` | `*int64` | AS number to prepend. Defaults to local ASN when `prepend` is set. |
| `replace` | `[]int64` | Replaces the entire AS path with the given list. Mutually exclusive with `prepend`. Max 32. |

**NextHopSet**

| Field | Type | Description |
|-------|------|-------------|
| `self` | `*bool` | Sets the next-hop to the local router's BGP peer address. Mutually exclusive with `address`. |
| `address` | `*string` | Sets the next-hop to a specific IP address. Mutually exclusive with `self`. |

**ExtendedCommunitySet**

| Field | Type | Description |
|-------|------|-------------|
| `add` | `[]string` | Extended communities to attach. Max 32. |
| `remove` | `[]string` | Extended communities to strip. Max 32. |

#### Validation

- Term `sequence` numbers must be unique within a policy (CEL enforced).
- `set` actions must not be specified on `deny` terms (CEL enforced).
- When `any` is `true`, all other match fields must be empty (CEL enforced).
- `asPath.prepend` and `asPath.replace` are mutually exclusive (CEL enforced).
- `nextHop.self` and `nextHop.address` are mutually exclusive (CEL enforced).
- `prefixList` entries must be valid CIDR notation (CEL enforced).
- `communityMatch` entries must be in `ASN:NN` or `IP:NN` format (CEL enforced).

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

```yaml
# EVPN selective import — accept MAC/IP and IP-prefix routes for VNI 10100
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPolicy
metadata:
  name: evpn-vni-10100-import
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
        evpnRouteType:
          - macIPAdvertisement
        vni: 10100
      action: permit
      set:
        communities:
          add: ["65000:10100"]
        extCommunities:
          add: ["65000:10100"]
    - sequence: 20
      match:
        addressFamilies:
          - afi: l2vpn
            safi: evpn
        evpnRouteType:
          - iPPrefixAdvertisement
        vni: 10100
      action: permit
    - sequence: 9999
      match:
        any: true
      action: deny
```

```yaml
# Export policy — tag routes by AS path and set SRv6 color for traffic engineering
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPolicy
metadata:
  name: srv6-te-export
  namespace: default
spec:
  routerRef:
    name: leaf-1-underlay
  direction: export
  terms:
    - sequence: 10
      match:
        prefixList:
          - "10.0.0.0/8"
          - "172.16.0.0/12"
      action: permit
      set:
        color: 100
        srv6EndpointBehavior: "End.DT6"
        asPath:
          prepend: 2
    - sequence: 20
      match:
        asPathFilter:
          pattern: "^65001"
          matchType: contains
      action: permit
      set:
        nextHop:
          self: true
        origin: igp
    - sequence: 9999
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

## 5. Known Limitations

### `uint32` fields rendered as `format: int32` in CRD schema

Fields typed as `uint32` in Go (e.g., `localASN`, `peerASN`, `localPreference`) are
rendered by controller-gen as `format: int32` in the OpenAPI schema due to a known
kubebuilder limitation. The real guardrails are the explicit `minimum` and `maximum`
constraints on each field — those are set correctly. The `format: int32` annotation
is advisory metadata only and does not restrict admission.

---

## 6. Common Verification Commands

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
