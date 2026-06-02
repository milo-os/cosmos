# FRR Provider

FRR version: 10.0+

Required FRR daemons: `zebra`, `bgpd`

## Implementation status

The FRR northbound gRPC client bindings are not currently vendored in this
module (`go.mod` does not include an FRR gRPC dependency). **All operations use
`vtysh`** — the FRR management shell — invoked as a subprocess via
`exec.CommandContext`. This is a functional but not optimal path; see the
migration note below.

## Operation routing

| Operation                | Method | Notes |
|--------------------------|--------|-------|
| `ConfigureSpeaker`       | vtysh  | Emits `router bgp`, `bgp router-id`, `timers bgp`, best-path knobs, cluster-id, address-family blocks |
| `AddOrUpdatePeer`        | vtysh  | `neighbor … remote-as`, timers, password, passive, allowas-in, route-reflector-client, ebgp-multihop, ttl-security, per-AF activation |
| `DeletePeer`             | vtysh  | `no neighbor <addr>` — no-op when peer absent |
| `AddOrUpdateAdvertisement` | vtysh | `network <prefix>` in the matching address-family block |
| `DeleteAdvertisement`    | vtysh  | `no network <prefix>` |
| `AddOrUpdatePolicy`      | vtysh  | `ip`/`ipv6 prefix-list` for conditions, `route-map … permit/deny` clauses with set actions |
| `DeletePolicy`           | vtysh  | `no route-map <name>` for both import and export maps |
| `Ready`                  | vtysh  | `show version` — exits 0 when `bgpd` is running |
| `Capabilities`           | static | Compile-time constants; no daemon query needed |

## vtysh commands

### ConfigureSpeaker

```
configure terminal
  router bgp <ASN>
    bgp router-id <RouterID>
    timers bgp <keepalive> <hold>
    [bgp always-compare-med]
    [bgp deterministic-med]
    [bgp bestpath compare-routerid]
    [bgp cluster-id <ClusterID>]
    [address-family ipv6 unicast]
    [exit-address-family]
  exit
end
write memory
```

### AddOrUpdatePeer

```
configure terminal
  router bgp <ASN>
    neighbor <addr> remote-as <peer-ASN>
    [neighbor <addr> timers <keepalive> <hold>]
    [neighbor <addr> password <password>]
    [neighbor <addr> passive]
    [neighbor <addr> allowas-in <count>]
    [neighbor <addr> route-reflector-client]
    [neighbor <addr> ebgp-multihop <ttl>]
    [neighbor <addr> ttl-security hops <hops>]
    address-family ipv6 unicast
      neighbor <addr> activate
    exit-address-family
  exit
end
write memory
```

### DeletePeer

```
configure terminal
  router bgp <ASN>
    no neighbor <addr>
  exit
end
write memory
```

### AddOrUpdateAdvertisement

```
configure terminal
  router bgp <ASN>
    address-family ipv6 unicast
      network <prefix>
    exit-address-family
  exit
end
write memory
```

IPv4 prefixes use `address-family ipv4 unicast` instead.

### DeleteAdvertisement

```
configure terminal
  router bgp <ASN>
    address-family ipv6 unicast
      no network <prefix>
    exit-address-family
  exit
end
write memory
```

### AddOrUpdatePolicy

```
configure terminal
  [ipv6 prefix-list bgp-<policy>-import-stmt0 seq 10 permit <cidr>]
  [ipv6 prefix-list bgp-<policy>-export-stmt0 seq 10 permit <cidr>]
  route-map bgp-<policy>-import permit 10
    [match ipv6 address prefix-list bgp-<policy>-import-stmt0]
    [set local-preference <value>]
    [set metric <value>]
    [set ipv6 next-hop global <addr>]
    [set community <communities>]
  exit
  route-map bgp-<policy>-export permit 10
    ...
  exit
end
write memory
```

### DeletePolicy

```
configure terminal
  no route-map bgp-<policy>-import
  no route-map bgp-<policy>-export
end
write memory
```

### Ready

```
vtysh -c "show version"
```

## Northbound gRPC migration path

When FRR northbound gRPC bindings are added to `go.mod`, migrate operations in
this priority order (highest coverage / lowest risk first):

1. **`Ready`** — use the gRPC health check service (`grpc.health.v1`)
2. **`ConfigureSpeaker`** — FRR northbound covers `bgpd` global config via
   `frr-northbound.yang` BGP module
3. **`AddOrUpdatePeer` / `DeletePeer`** — neighbor YANG nodes have full gRPC
   coverage in FRR 10+
4. **`AddOrUpdateAdvertisement` / `DeleteAdvertisement`** — `network` statements
   are in the BGP YANG tree
5. **`AddOrUpdatePolicy` / `DeletePolicy`** — route-map coverage in the
   routing-policy YANG module; migrate last as it is the most complex

Do not add a `FRRNorthboundClient` struct or partial gRPC paths until the
bindings are available and all targeted operations can be migrated atomically
within a single implementation version.

## Naming conventions

| FRR object      | Name pattern                              |
|-----------------|-------------------------------------------|
| route-map (in)  | `bgp-<policyName>-import`                 |
| route-map (out) | `bgp-<policyName>-export`                 |
| prefix-list     | `bgp-<policyName>-<direction>-stmt<N>`    |
