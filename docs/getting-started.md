# Getting Started

This guide walks through the minimal resources needed to bring up a POP node
with cosmos managing two BGP planes.

By the end of this guide you will have:

- A BGPRouter for the underlay plane (IPv6 unicast)
- A BGPRouter for the overlay plane (L2VPN EVPN)
- A BGPPeer peering the underlay to a top-of-rack switch
- A BGPPeer peering the overlay to a regional route reflector

---

## Before you begin

You need:

- A Kubernetes cluster representing a POP node with IPv6 enabled
- `kubectl` configured to reach the cluster
- An infra cluster with a running route reflector (BGPRouter configured as a route reflector)
- A management cluster with Karmada running, from which BGPPeer resources
  are propagated

---

## Step 1: Install the CRDs

Apply the CRD manifests:

```bash
kubectl apply -k config/crd
```

Verify the API group is installed:

```bash
kubectl get crds | grep miloapis.com
```

You should see:

```
bgpadvertisements.bgp.miloapis.com
bgpexternalpeers.bgp.miloapis.com
bgppeers.bgp.miloapis.com
bgproutepolicies.bgp.miloapis.com
bgprouters.bgp.miloapis.com
bgpvrfinstances.bgp.miloapis.com
vpcs.vpc.miloapis.com
vpcattachments.vpc.miloapis.com
```

---

## Step 2: Create the underlay BGPRouter

Create a BGPRouter to represent the underlay routing context for IPv6 unicast:

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

Apply:

```bash
kubectl apply -f underlay-router.yaml
```

Verify the router is accepted:

```bash
kubectl get bgprouter node-1-underlay
```

---

## Step 3: Create the overlay BGPRouter

Create a BGPRouter to represent the overlay routing context for L2VPN EVPN:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPRouter
metadata:
  name: node-1-overlay
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
    - afi: l2vpn
      safi: evpn
```

Apply:

```bash
kubectl apply -f overlay-router.yaml
```

---

## Step 4: Create BGPPeer resources in the management cluster

BGPPeer resources are written in the management cluster and propagated to
POP/infra clusters via Karmada.

### Underlay peer (node to ToR)

```yaml
# Propagated by Karmada to the POP cluster.
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeer
metadata:
  name: node-1-to-tor-1
  namespace: default
spec:
  routerRef:
    name: node-1-underlay
  address: "2001:db8:fabric::1"
  peerASN: 65000
  addressFamilies:
    - afi: ipv6
      safi: unicast
```

### Overlay peer (node to route reflector)

```yaml
# Propagated by Karmada to the POP cluster.
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeer
metadata:
  name: node-1-overlay-to-rr-apac-1
  namespace: default
spec:
  routerRef:
    name: node-1-overlay
  address: "2001:db8:0:rr-apac-1::1"
  peerASN: 65000
  description: "rr-apac-1"
  addressFamilies:
    - afi: l2vpn
      safi: evpn
  holdTime: 90s
  keepaliveTime: 30s
```

Apply to the management cluster:

```bash
kubectl --context management apply -f underlay-peer.yaml
kubectl --context management apply -f overlay-peer.yaml
```

Karmada propagates the BGPPeer resources to the POP cluster.

---

## Step 5: Verify BGPPeer resources

After Karmada propagation, verify that BGPPeer resources exist on the POP cluster:

```bash
kubectl get bgppeers
```

Check session state:

```bash
kubectl get bgppeers -o wide
```

The `STATE` printer column reflects the BGP FSM state (`Idle`, `Active`,
`Established`, etc.) as reported by the implementation. If a session is not
establishing:

1. Verify network reachability between the node and the peer address
2. Check that the remote BGP agent is listening on the expected port
3. Check the conditions on the BGPPeer:
   ```bash
   kubectl describe bgppeer <peer-name>
   ```

---

## Step 6: Advertise infrastructure prefixes (underlay only)

For loopback addresses and SRv6 locator blocks, create a BGPAdvertisement
on the POP cluster:

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
```

Apply:

```bash
kubectl apply -f advertisement.yaml
```

Verify the advertisement:

```bash
kubectl get bgpadvertisement node-1-loopback
```

---

## Step 7: Apply route policy (optional)

To control which routes are advertised to peers, create a BGPRoutePolicy:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPRoutePolicy
metadata:
  name: node-1-underlay-export
  namespace: default
spec:
  routerRef:
    name: node-1-underlay
  direction: export
  terms:
    - sequence: 10
      match:
        addressFamilies:
          - afi: ipv6
            safi: unicast
      action: permit
    - sequence: 20
      match:
        any: true
      action: deny
```

Apply:

```bash
kubectl apply -f route-policy.yaml
```

---

## Next steps

- Read the full [API reference](api/) for all CRDs, including field
  definitions and conditions.
- Review the [example files](examples/) for complete annotated YAML:
  - [BGPExternalPeer — ToR switch](examples/mgmt-bgpexternalpeer-tor.yaml)
  - [Rejected configurations](examples/rejected-configurations.yaml)

<!-- References -->
[bgp]: https://datatracker.ietf.org/doc/html/rfc4271
