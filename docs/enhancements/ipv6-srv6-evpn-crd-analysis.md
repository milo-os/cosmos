# IPv6 SRv6 EVPN CRD Analysis & Recommendations

**Status:** Draft
**Date:** 2026-06-22
**Scope:** All CRDs in `bgp.miloapis.com/v1alpha1` and `vpc.miloapis.com/v1alpha1`

---

## 1. Inventory

| CRD | Scope | Purpose |
|-----|-------|---------|
| `BGPRouter` | Namespaced | BGP routing context: ASN, router ID, address families, roles |
| `BGPPeer` | Namespaced | BGP session to a remote peer |
| `BGPAdvertisement` | Namespaced | Prefix advertisement from a single router |
| `BGPPolicy` | Namespaced | Import/export route filtering with ordered terms |
| `BGPVRFInstance` | Namespaced | L2VPN EVPN VRF: RD, import/export RTs |
| `VPC` | Namespaced | Virtual network with IPv4/IPv6 CIDR prefixes |
| `VPCAttachment` | Namespaced | Binds a VPC to a named network interface |

**Relationship:** `BGPRouter` is the anchor — all other BGP CRDs reference it via `routerRef` (single) or `routerSelector` (multiple). `BGPAdvertisement` deliberately only supports `routerRef`.

---

## 2. What's Already Good

The API has a solid foundation:

- **Clean ownership model** — `BGPRouter` as the anchor with `routerRef`/`routerSelector` targeting is well-designed
- **CEL validation** — server-side validation for AFI/SAFI combos, IP/CIDR format, unique sequences, mutual exclusions
- **Standard conditions** — `Ready` and `Accepted` condition types with reference implementations
- **BGP FSM tracking** — `sessionState` enum covers all 6 states with Idle sub-reasons
- **Address family abstraction** — AFI/SAFI struct prevents invalid combinations at admission time
- **Policy model** — ordered terms with match/set semantics is BGP-accurate
- **v1alpha1** — appropriate for an API that will evolve; no premature v1 commitment

---

## 3. Gaps for IPv6 SRv6 EVPN

### 3.1 BGPVRFInstance — Missing EVPN Core Fields

**Critical gap.** The current `BGPVRFInstance` only has RD and RTs. A production EVPN VRF needs:

| Missing Field | Why It Matters |
|---|---|
| `vni` (uint32) | The VNI is the primary L2 segmentation identifier in EVPN. Without it, the VRF has no data-plane identity. RFC 7432 requires EVPN routes to carry a Route Distinguisher that includes the VNI. |
| `autoRouteTarget` (bool) | Auto-RT discovery (RFC 8247) derives RTs from the VNI and RD automatically. Most modern EVPN deployments use this. Manual RT configuration per VRF doesn't scale. |
| `importRouteTargets` / `exportRouteTargets` as optional | With auto-RT, these should not be required. |
| `routeDistinguisher` derivation | RDs should be derivable from node IP + VNI (e.g., `RouterID:VNI`), making manual specification optional. |

**Recommendation:** Make `importRouteTargets` and `exportRouteTargets` optional. Add `vni` as a required field when `l2vpn-evpn` is used. Add `autoRouteTarget` boolean. Derive default RD from `BGPRouter.routerID` + VNI.

### 3.2 No SRv6 Support at All

This is the largest gap. SRv6 requires several new resource types:

#### 3.2.1 SRv6 SID Management

SRv6 uses SIDs (Segment Identifiers — 128-bit IPv6 addresses) as network programming tokens. The API needs:

```go
type SRv6SIDBlock struct {
    // Prefix is the SRv6 SID prefix (e.g., "2001:db8:sid::/48")
    Prefix string `json:"prefix"`
    // Length is the SID block length in bits (48-128, default 48)
    Length int32 `json:"length,omitempty"`
    // NodeSID is the local node's SID within this block
    NodeSID *string `json:"nodeSid,omitempty"`
}
```

**Where it lives:** Either as a new `SRv6SIDBlock` CRD or as a field on `BGPRouter.Spec`. Since SIDs are node-local, embedding in `BGPRouter` makes sense:

```yaml
spec:
  srv6:
    enabled: true
    sidBlock:
      prefix: "2001:db8:sid::"
      length: 48
    nodeSID: "2001:db8:sid::1"
```

#### 3.2.2 SRv6 Policy CRD

SRv6 policies define segment lists (ordered lists of SIDs):

```go
type SRv6Policy struct {
    // EndpointBehavior is the SRv6 endpoint behavior.
    // Enum: End, End.X, End.DX2, End.DT6, End.UX6, End.B6, End.M
    EndpointBehavior string `json:"endpointBehavior"`
    // Preference is the policy preference (lower = higher priority).
    Preference int32 `json:"preference"`
    // Segments is the ordered list of SIDs forming the segment list.
    Segments []SRv6Segment `json:"segments"`
    // BindingType determines if this is a global policy or VRF-bound.
    BindingType string `json:"bindingType"` // "global" | "vrf"
    // VRFRef references a BGPVRFInstance when bindingType=vrf.
    VRFRef *RouterRef `json:"vrfRef,omitempty"`
}

type SRv6Segment struct {
    // Address is the 128-bit IPv6 SID.
    Address string `json:"address"` // validated with isIPv6(self)
    // Adapter is optional SRv6 adapter identifier (SID set index).
    Adapter *string `json:"adapter,omitempty"`
}
```

#### 3.2.3 SRv6 Transport Policy

For SRv6-based underlay (IP-in-IPv6 tunneling between nodes):

```go
type SRv6TransportPolicy struct {
    // Source is the local node SID prefix.
    Source string `json:"source"`
    // Destination is the remote node SID.
    Destination string `json:"destination"`
    // EndDX6 indicates decapsulation + L3 forwarding at the endpoint.
    EndDX6 bool `json:"endDX6,omitempty"`
    // EndDX2 indicates decapsulation + L2 forwarding at the endpoint.
    EndDX2 bool `json:"endDX2,omitempty"`
}
```

### 3.3 BGPPeer — Missing IPv6-First and EVPN Fields

| Missing Field | Why It Matters |
|---|---|
| `holdTimer` / `keepaliveTimer` per-AFI | In EVPN, different AFI/SAFI may need different timers. Not strictly required but useful. |
| `multiSession` (bool) | BFD over MP-BGP (RFC 8935) requires multi-session BGP. Each AFI/SAFI gets its own BFD session. |
| `bfd` (BFD config struct) | BFD is essential for sub-500ms failure detection in EVPN. Without it, convergence relies on hold timers (typically 90s). |
| `password` (plain text, optional) | MD5/TCP-AO auth via Secret is good, but some implementations need plain-text for simplicity in non-prod. |
| `ebgpMultiHop` (bool) | Peers not on directly connected links. Required for spine-leaf or routed underlay. |
| `routeMapIn` / `routeMapOut` | Policy attachment per-peer direction. Currently policies are router-scoped only. |
| `nextHopSelf` (bool) | Common in EVPN iBGP peering. |
| `removePrivateAS` (uint32) | Strip private ASNs on eBGP export. |
| `defaultOriginRoute` (string) | Default route origination for EVPN. |
| `gracefulRestart` (bool + timers) | BGP graceful restart (RFC 4724) is critical for EVPN to avoid MAC/IP flapping during control-plane restarts. |

### 3.4 BGPPolicy — Limited Match/MatchSet Actions

The current policy model is minimal. For a production EVPN/SRv6 deployment:

| Missing Match Condition | Why It Matters |
|---|---|
| `prefixList` (name or inline) | Match routes against named/inline prefix-lists. Essential for EVPN route filtering. |
| `asPathFilter` (name or inline) | Match/filter by AS path. Used for loop prevention and traffic engineering. |
| `communityMatch` | Match routes by BGP community. Critical for EVPN route selection (e.g., prefer specific RTs). |
| `routeType` (EVPN route type enum) | Match Type-1 (host route), Type-2 (MAC/IP), Type-3 (IP prefix), Type-4 (sticky), Type-5 (IP prefix). Needed for selective EVPN route import. |
| `vni` (uint32) | Match by VNI in EVPN routes. |
| `macAddress` (string) | Match MAC address in EVPN Type-2 routes. |
| `ipPrefix` (cidr) | Match specific IP prefixes inline. |
| `localPref` (match) | Match routes by LOCAL_PREF. |
| `med` (match) | Match routes by MED. |

| Missing Set Action | Why It Matters |
|---|---|
| `origin` (igp | egp | incomplete) | Set BGP origin attribute. |
| `asPath` (prepend | set) | Prepend AS path or set it. |
| `nextHop` (self | address) | Override next-hop. Critical for EVPN. |
| `originatorID` | Set BGP originator ID (for route reflection). |
| `clusterList` | Set BGP cluster list (for route reflection). |
| `extCommunity` (add/remove) | Extended communities beyond standard communities. Needed for EVPN RT manipulation. |
| `metric` (MED) | Set MED for traffic engineering. |
| `color` (uint32) | Set SRv6 policy color for path selection. |
| `srv6EndpointBehavior` | Set SRv6 endpoint behavior on a route. |

### 3.5 BGPAdvertisement — Per-Prefix Attributes Missing

Current model advertises a set of prefixes with uniform attributes. For SRv6/EVPN:

| Missing Feature | Why It Matters |
|---|---|
| Per-prefix communities/localPref | Different prefixes may need different communities (e.g., loopback /128 vs. subnet routes). |
| `originateFrom` (interface/route) | Originate routes from local interface addresses or kernel routes rather than static prefixes. |
| `routeMap` (name) | Apply a route-map before advertisement for conditional origination. |
| `redistribute` (static | connected | kernel) | Redistribute local routing table entries into BGP. |

### 3.6 Route Target Format Too Restrictive

Current regex only allows `ASN:NN` or `IP:NN`. RFC 6514 defines three RD/RT formats:

1. **Type 0:** `ASN:NN` (2-byte ASN : 16-bit number) — max ASN 65535
2. **Type 1:** `ASN:NN` (4-byte ASN : 16-bit number) — supports full 32-bit ASN range
3. **Type 2:** `IP:NN` (4-octet IP : 16-bit number)

The current regex `[0-9]{1,9}` for the first part allows up to 9 digits, which is fine for both 2-byte and 4-byte ASNs. However, the IP format regex `[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}` doesn't validate that each octet is 0-255.

**Recommendation:** Add per-octet validation or switch to a format that uses `isIP()` for the IP portion. Also document that Type 0 (2-byte ASN) uses the `+kubebuilder:validation:Maximum=65535` constraint where applicable.

### 3.7 KeepaliveTime Validation Missing

`HoldTime` has CEL validation (`duration(self) == duration('0s') || duration(self) >= duration('3s')`), but `KeepaliveTime` has no validation. The BGP spec (RFC 4271) requires `keepalive <= holdTime / 3`.

**Recommendation:** Add CEL validation. Since CEL can't express `self <= other / 3` easily with `metav1.Duration`, consider:
- Adding a `+kubebuilder:validation:XValidation` that checks `duration(self) <= duration('30s')` with a default-based constraint
- Or documenting the constraint strongly and enforcing it in the controller with an `Accepted=False` condition

### 3.8 BGPRouter — RouterID IPv4-Only

The `routerID` field uses `format: ipv4` (string format validation). In an IPv6-only underlay, this is a logical identifier only — the comment acknowledges this.

**Recommendation:** Allow IPv6 RouterIDs. Use a CEL rule:

```go
// +kubebuilder:validation:XValidation:rule="isIP(self) || self.matches('^[0-9]{1,3}\\\\.[0-9]{1,3}\\\\.[0-9]{1,3}\\\\.[0-9]{1,3}$')",message="routerID must be a valid IPv4 or IPv6 address"
```

Or simply: `isIP(self)` — but note that `isIP()` in Kubernetes CEL accepts both IPv4 and IPv6, so this would work.

### 3.9 No BFD Support

BFD (Bidirectional Forwarding Detection) is critical for fast failure detection in EVPN deployments. Without BFD, convergence relies on BGP hold timers (default 90s), which is unacceptable for production.

**Recommendation:** Add a `BGPPeerBFD` type:

```go
type BGPPeerBFD struct {
    // Enabled indicates whether BFD is enabled for this peer.
    Enabled bool `json:"enabled"`
    // MinimumTX is the minimum transmission interval (microseconds).
    MinimumTX *metav1.Duration `json:"minimumTx,omitempty"`
    // MinimumRX is the minimum reception interval (microseconds).
    MinimumRX *metav1.Duration `json:"minimumRx,omitempty"`
    // MultiHop enables BFD for eBGP multi-hop sessions.
    MultiHop bool `json:"multiHop,omitempty"`
}
```

### 3.10 No BGP Graceful Restart

Graceful restart (RFC 4724) is essential for EVPN to prevent MAC/IP flapping during controller/router restarts. Without it, a brief control-plane restart causes wholesale MAC route withdrawal and re-advertisement, triggering ARP storms.

**Recommendation:** Add to `BGPRouter.Spec`:

```go
type BGPRouterGracefulRestart struct {
    // Enabled indicates whether graceful restart is enabled.
    Enabled bool `json:"enabled"`
    // RestartTime is the maximum time to wait for recovery (seconds, 1-1200).
    RestartTime *uint32 `json:"restartTime,omitempty"`
    // StaleRouteTime is the maximum time to keep stale routes (seconds).
    StaleRouteTime *uint32 `json:"staleRouteTime,omitempty"`
    // HelperOnly restricts GR to helper role (don't restart own routes).
    HelperOnly bool `json:"helperOnly,omitempty"`
}
```

### 3.11 VPCAttachment — Weird Default

The `VPCAttachmentInterface.Name` field has `+kubebuilder:validation:XValidation` with a default of `"galactic0"`. This looks like a placeholder/test value that should not be in production API:

```go
// +default:value="galactic0"
Name string `json:"name"`
```

**Recommendation:** Remove the `galactic0` default. If a default is needed, use an empty string or a more meaningful default like `"eth0"`.

### 3.12 No VRF-to-VRF Route Leak Control

In multi-tenant EVPN, VRFs need controlled route leakage (VRF leaking). The current API has no mechanism to define which VRFs can leak routes to which other VRFs.

**Recommendation:** Add a `VRFLeak` CRD or a `leaks` field on `BGPVRFInstance`:

```go
type VRFLeak struct {
    // SourceVRF references the source VRF.
    SourceVRF RouterRef `json:"sourceVrf"`
    // DestinationVRF references the destination VRF.
    DestinationVRF RouterRef `json:"destinationVrf"`
    // RouteTargetMapping defines how RTs are translated.
    RouteTargetMapping []RouteTargetMapping `json:"routeTargetMapping,omitempty"`
}

type RouteTargetMapping struct {
    Source string `json:"source"`  // RT in source VRF
    Target string `json:"target"`  // RT in destination VRF
}
```

### 3.13 No EVPN-Specific Route Attributes in Policy

EVPN has 5 route types, each with different semantics:

| Type | Name | Use |
|------|------|-----|
| 1 | Inclusive Multicast Ethernet Tag | BUM traffic distribution |
| 2 | MAC-IP Advertisement | Host reachability |
| 3 | IP Prefix Advertisement | L3 route distribution |
| 4 | Sticky MAC Address | MAC mobility |
| 5 | IPv6 Prefix Advertisement | IPv6 L3 routes |

The current `BGPPolicyMatch` has no way to match on EVPN route type, VNI, MAC address, or IP prefix.

**Recommendation:** Extend `BGPPolicyMatch`:

```go
type BGPPolicyMatch struct {
    // ... existing fields ...

    // EVPNRouteType matches specific EVPN route types.
    EVPNRouteType []EVPNRouteType `json:"evpnRouteType,omitempty"`

    // VNI matches routes by VNI.
    VNI *uint32 `json:"vni,omitempty"`

    // MACAddress matches MAC-IP routes by MAC.
    MACAddress *string `json:"macAddress,omitempty"`

    // IPPrefix matches IP prefix routes.
    IPPrefix *string `json:"ipPrefix,omitempty"`
}

type EVPNRouteType string

const (
    EVPNRouteTypeInclusiveMulticastTag EVPNRouteType = "InclusiveMulticastEthernetTag"
    EVPNRouteTypeMACIPAdvertisement    EVPNRouteType = "MACIPAdvertisement"
    EVPNRouteTypeIPPrefixAdvertisement EVPNRouteType = "IPPrefixAdvertisement"
    EVPNRouteTypeStickyMACAddress      EVPNRouteType = "StickyMACAddress"
    EVPNRouteTypeIPv6PrefixAdvertisement EVPNRouteType = "IPv6PrefixAdvertisement"
)
```

### 3.14 No Peer Group Support

BGP peer groups (RFC 4271) are critical for scalability. Without them, each peer is configured individually, which doesn't scale to hundreds of peers (common in spine-leaf or large fabric).

**Recommendation:** Add a `BGPPeerGroup` CRD:

```go
type BGPPeerGroup struct {
    // Peers is the list of peer specifications within this group.
    Peers []BGPPeerGroupPeer `json:"peers"`
    // RemoteASN is the common ASN for all peers in this group.
    RemoteASN int64 `json:"remoteASN"`
    // Description is a human-readable label.
    Description string `json:"description,omitempty"`
}

type BGPPeerGroupPeer struct {
    // Address is the peer's IP address.
    Address string `json:"address"`
    // Description is a per-peer override.
    Description string `json:"description,omitempty"`
}
```

### 3.15 Status Gaps

Several status fields are missing that would be valuable for operations:

| Missing Status Field | Resource | Why |
|---|---|---|
| `lastStateChange` | BGPPeer | When did the session last change state? |
| `uptime` | BGPPeer | How long has the session been up? |
| `prefixesReceived` | BGPPeer | Count of received prefixes per AFI/SAFI |
| `prefixesAdvertised` | BGPPeer | Count of advertised prefixes per AFI/SAFI |
| `messagesSent` / `messagesReceived` | BGPPeer | BGP message counters for troubleshooting |
| `afiSafi` | BGPPeer | Per-AFI/SAFI negotiation status |
| `vni` | BGPVRFInstance | Echo the VNI in status for quick lookup |
| `evpnRouteCount` | BGPVRFInstance | Total EVPN routes per type |
| `srv6PolicyCount` | SRv6Policy (new) | Number of active SRv6 policies |
| `appliedSegments` | SRv6Policy (new) | Number of active segment list entries |

### 3.16 No Label/Route Distinguisher Uniqueness Constraint

There's no mechanism to prevent duplicate RDs across VRFs on the same router. Duplicate RDs cause route leaking between VRFs — a serious data-plane bug.

**Recommendation:** Add a CEL validation on `BGPVRFInstance` that checks for uniqueness within the namespace (or cluster-scoped if feasible):

```yaml
+x-kubernetes-validations:
  - rule: "self.routeDistinguisher !=" (this is hard to enforce at CRD level;
    better handled by controller with Accepted=False condition)
```

### 3.17 No Cluster-Scoped Resources

Some resources (like SRv6 SID blocks, peer groups) are naturally cluster-scoped. All current resources are Namespaced, which limits their utility.

**Recommendation:** Consider making `SRv6SIDBlock` and `BGPPeerGroup` cluster-scoped.

### 3.18 VPC API — Missing EVPN Integration

The VPC API (`vpc.miloapis.com/v1alpha1`) is decoupled from the BGP API. For an EVPN deployment, VPCs need to be advertised into BGP (via `BGPAdvertisement`) and associated with EVPN VRFs.

**Recommendation:** Add a cross-reference field:

```go
type VPCSpec struct {
    Networks []string `json:"networks"`
    // BGPAdvertisementRef optionally references a BGPAdvertisement
    // that originates these networks.
    BGPAdvertisementRef *RouterRef `json:"bgpAdvertisementRef,omitempty"`
    // VRFInstanceRef optionally binds this VPC to a BGPVRFInstance.
    VRFInstanceRef *RouterRef `json:"vrfInstanceRef,omitempty"`
}
```

---

## 4. Recommended CRD Additions (New Types)

| New CRD | Scope | Purpose |
|---------|-------|---------|
| `SRv6SIDBlock` | Cluster | SRv6 SID prefix allocation per node |
| `SRv6Policy` | Namespaced | SRv6 segment-list policies |
| `BGPPeerGroup` | Cluster | Scalable peer group definitions |
| `VRFLeak` | Namespaced | Controlled route leakage between VRFs |

---

## 5. Prioritized Implementation Plan

### P0 — Blockers for EVPN Production

1. **Add `vni` to `BGPVRFInstance`** — required for any EVPN data-plane operation
2. **Add `bfd` to `BGPPeer`** — required for sub-second failure detection
3. **Add `gracefulRestart` to `BGPRouter`** — required to prevent flapping during restarts
4. **Add `evpnRouteType` / `vni` / `macAddress` match to `BGPPolicyMatch`** — required for EVPN route filtering
5. **Add `nextHopSelf` to `BGPPeer`** — required for iBGP EVPN peering

### P1 — Important for SRv6

6. **Add `srv6` block to `BGPRouter`** — SID block + node SID
7. **Create `SRv6Policy` CRD** — segment-list policies
8. **Add `color` set action to `BGPPolicy`** — SRv6 path selection
9. **Add `extCommunity` (add/remove) to `PolicySetActions`** — EVPN RT manipulation
10. **Add `routeType` set action** — BGP origin attribute

### P2 — Operations & Scalability

11. **Add `BGPPeerGroup` CRD** — peer scalability
12. **Add status fields** — uptime, prefix counts, message counters
13. **Add `bgpAdvertisementRef` / `vrfInstanceRef` to VPC** — cross-API integration
14. **Add `autoRouteTarget` to `BGPVRFInstance`** — RT automation
15. **Allow IPv6 RouterID** — IPv6-only underlay support
16. **Add `ebgpMultiHop` to `BGPPeer`** — non-direct peering

### P3 — Nice to Have

17. **Add `VRFLeak` CRD** — multi-tenant route control
18. **Add `redistribute` to `BGPAdvertisement`** — dynamic route origination
19. **Add `multiSession` to `BGPPeer`** — BFD over MP-BGP
20. **Add `asPathFilter` / `prefixList` match to `BGPPolicy`** — richer policy matching
21. **Fix `keepaliveTime` validation** — CEL enforcement
22. **Fix `RouteTarget` format validation** — per-octet IP validation

---

## 6. Example: Complete IPv6 SRv6 EVPN Configuration

```yaml
# --- SRv6 SID allocation ---
apiVersion: bgp.miloapis.com/v1alpha1
kind: SRv6SIDBlock
metadata:
  name: fabric-sid-block
spec:
  prefix: "2001:db8:sid::"
  length: 48
---
# --- BGPRouter with SRv6 ---
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPRouter
metadata:
  name: leaf-1-underlay
  namespace: fabric
  labels:
    bgp.miloapis.com/role: fabric
spec:
  targetRef:
    kind: Node
    name: leaf-1
  roles:
    - fabric
  localASN: 65000
  routerID: "10.255.0.1"
  gracefulRestart:
    enabled: true
    restartTime: 120
    staleRouteTime: 300
  srv6:
    enabled: true
    sidBlock:
      prefix: "2001:db8:sid::"
      length: 48
    nodeSID: "2001:db8:sid::1"
  addressFamilies:
    - afi: ipv6
      safi: unicast
    - afi: l2vpn
      safi: evpn
---
# --- BGPPeer with BFD ---
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeer
metadata:
  name: leaf-1-to-spine-1
  namespace: fabric
spec:
  routerRef:
    name: leaf-1-underlay
  peerASN: 65000
  address: "2001:db8:fabric::2"
  description: "spine-1"
  ebgpMultiHop: false
  bfd:
    enabled: true
    minimumTx: 300ms
    minimumRx: 300ms
    detectMultiplier: 3
  gracefulRestart:
    enabled: true
  addressFamilies:
    - afi: ipv6
      safi: unicast
    - afi: l2vpn
      safi: evpn
---
# --- EVPN VRF with VNI ---
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPVRFInstance
metadata:
  name: tenant-alpha
  namespace: fabric
spec:
  routerSelector:
    matchLabels:
      bgp.miloapis.com/role: tenant
  routeDistinguisher: "10.255.0.1:100"
  vni: 10100
  autoRouteTarget: true
  importRouteTargets:
    - value: "65000:10100"
  exportRouteTargets:
    - value: "65000:10100"
---
# --- SRv6 Policy: End.X between leaf-1 and leaf-2 ---
apiVersion: bgp.miloapis.com/v1alpha1
kind: SRv6Policy
metadata:
  name: leaf-1-to-leaf-2-endx
  namespace: fabric
spec:
  routerRef:
    name: leaf-1-underlay
  endpointBehavior: End.X
  preference: 100
  segments:
    - address: "2001:db8:sid::1"  # leaf-1 SID (source)
    - address: "2001:db8:sid::2"  # leaf-2 SID (destination)
  bindingType: global
---
# --- EVPN-aware policy ---
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPolicy
metadata:
  name: evpn-import-policy
  namespace: fabric
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
          - MACIPAdvertisement
        vni: 10100
      action: permit
      set:
        communities:
          add: ["65000:10100"]
    - sequence: 20
      match:
        addressFamilies:
          - afi: l2vpn
            safi: evpn
        evpnRouteType:
          - IPPrefixAdvertisement
        vni: 10100
      action: permit
      set:
        communities:
          add: ["65000:10100"]
    - sequence: 9999
      match:
        any: true
      action: deny
```

---

## 7. Summary of Changes by Category

| Category | Current State | Recommended State |
|----------|--------------|-------------------|
| **EVPN VRF** | RD + RTs only | + VNI, auto-RT, route-type matching |
| **SRv6** | None | + SID blocks, policies, segment lists |
| **Peer BFD** | None | + per-peer BFD config |
| **Graceful Restart** | None | + per-router GR config |
| **Policy Match** | Any, AFI/SAFI only | + prefix-list, AS-path, community, route-type, VNI, MAC, IP prefix |
| **Policy Set** | Communities, localPref | + extCommunity, origin, as-path, next-hop, color, metric |
| **RouterID** | IPv4 only | IPv4 or IPv6 |
| **Keepalive validation** | Comment only | CEL enforcement |
| **Route Target format** | ASN:NN or IP:NN | Same, but fix IP octet validation |
| **Status** | Basic FSM + conditions | + uptime, prefix counts, message counters, per-AFI/SAFI |
| **Peer Groups** | None | + cluster-scoped BGPPeerGroup CRD |
| **VPC-BGP integration** | None | + cross-reference fields |
| **VRF Leaking** | None | + VRFLeak CRD or inline leaks |
| **VPCAttachment.Name default** | "galactic0" | Remove or change to "eth0" |
