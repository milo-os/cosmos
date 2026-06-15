# Getting Started

This guide walks through the minimal resources needed to bring up a POP node
running both the FRR underlay and GoBGP overlay BGP planes with cosmos.

By the end of this guide you will have:

- cosmos running on a POP node with auto-bootstrapped BGPProvider resources
- A BGPInstance for the FRR underlay (IPv6 unicast)
- A BGPInstance for the GoBGP overlay (VPNv4/VPNv6)
- A BGPPeer peering the underlay to a top-of-rack switch
- A BGPPeer peering the overlay to a regional route reflector

---

## Before you begin

You need:

- A Kubernetes cluster representing a POP node with IPv6 enabled
- `kubectl` configured to reach the cluster
- An infra cluster with a running route reflector (BGPInstance with
  `spec.routeReflector` set)
- A management cluster with Karmada running, from which BGPSession resources
  are propagated

---

## Step 1: Install the CRDs

Apply the CRD manifests:

```bash
kubectl apply -k config/crd
```

Verify that both API groups are installed:

```bash
kubectl get crds | grep miloapis.com
```

You should see six CRDs:

```
bgpadvertisements.bgp.miloapis.com
bgpexternalpeers.bgp.miloapis.com
bgpinstances.bgp.miloapis.com
bgppeers.bgp.miloapis.com
bgproutepolicies.bgp.miloapis.com
bgpproviders.providers.bgp.miloapis.com
```

---

## Step 2: Deploy the cosmos controller

Apply the full deployment:

```bash
kubectl apply -k config/deploy
```

This deploys the cosmos controller DaemonSet in the `bgp-system` namespace.
Each pod manages both the FRR (underlay) and GoBGP (overlay) daemons on the
node.

Verify the DaemonSet is running:

```bash
kubectl -n bgp-system get daemonset cosmos
kubectl -n bgp-system get pods -l app.kubernetes.io/name=cosmos
```

---

## Step 3: Verify BGPProvider auto-bootstrap

cosmos bootstraps one BGPProvider per daemon at startup. You do not create
these manually. After the DaemonSet pods become ready, verify that BGPProvider
resources exist:

```bash
kubectl get bgpproviders
```

You should see one provider per daemon per node. The labels identify the plane
and daemon type:

```
NAME            TYPE    READY
node-1-frr      FRR     True
node-1-gobgp    GoBGP   True
```

If a provider is not ready, check the conditions:

```bash
kubectl describe bgpprovider node-1-frr
```

The `Ready` condition will indicate whether the daemon is reachable on its
gRPC endpoint.

---

## Step 4: Create the underlay BGPInstance (FRR)

Create a BGPInstance to configure the FRR daemon as the underlay speaker.
The `providerSelector` targets the auto-bootstrapped FRR provider:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPInstance
metadata:
  name: underlay
spec:
  providerSelector:
    matchLabels:
      bgp.datum.net/plane: underlay
      bgp.miloapis.com/daemon: frr
  asNumber: 65000
  addressFamilies:
    - afi: IPv6
      safi: Unicast
```

Apply:

```bash
kubectl apply -f underlay-instance.yaml
```

Verify the instance is reconciled:

```bash
kubectl get bgpinstances underlay -o jsonpath='{.status.providers}'
```

The per-provider status should show a `Ready: True` condition for `node-1-frr`.

---

## Step 5: Create the overlay BGPInstance (GoBGP)

Create a BGPInstance to configure GoBGP as the overlay speaker:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPInstance
metadata:
  name: overlay
spec:
  providerSelector:
    matchLabels:
      bgp.datum.net/plane: overlay
      bgp.miloapis.com/daemon: gobgp
  asNumber: 65000
  addressFamilies:
    - afi: IPv4
      safi: VPNUnicast
    - afi: IPv6
      safi: VPNUnicast
```

Apply:

```bash
kubectl apply -f overlay-instance.yaml
```

> **Note:** The CNI plugin manages VRF instances and path injection on this
> GoBGP instance independently. Do not use BGPAdvertisement for overlay routes.
> See the Operational Contract in `docs/api/bgp.md` for the API ownership
> boundary between cosmos and the CNI plugin.

---

## Step 6: Create BGPPeer resources in the management cluster

BGPPeer resources are written in the management cluster and propagated to
POP/infra clusters via Karmada.

### Underlay peer (node to ToR)

```yaml
# Propagated by Karmada to the POP cluster based on providerSelector labels.
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeer
metadata:
  name: node-1-to-tor-1
spec:
  instanceRef: underlay
  providerSelector:
    matchLabels:
      bgp.miloapis.com/node: node-1
      bgp.miloapis.com/daemon: frr
  address: "2001:db8:fabric::1"
  asNumber: 65000
  addressFamilies:
    - afi: IPv6
      safi: Unicast
  allowAsIn: 1
```

### Overlay peer (node to route reflector)

```yaml
# Propagated by Karmada to the POP cluster.
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPPeer
metadata:
  name: tokyo-overlay-to-rr-apac-1
spec:
  instanceRef: overlay
  providerSelector:
    matchLabels:
      bgp.datum.net/plane: overlay
      bgp.datum.net/pop: jp-east-1
      bgp.miloapis.com/daemon: gobgp
  address: "2001:db8::rr-apac-1"
  asNumber: 65000
  addressFamilies:
    - afi: IPv4
      safi: VPNUnicast
    - afi: IPv6
      safi: VPNUnicast
  timers:
    holdTime: 90
    keepalive: 30
```

Apply to the management cluster:

```bash
kubectl --context management apply -f underlay-peer.yaml
kubectl --context management apply -f overlay-peer.yaml
```

Karmada propagates the BGPPeer resources to the POP cluster and the
PeerReconciler configures the daemons.

---

## Step 7: Verify BGPPeer resources and session state

After Karmada propagation and reconciliation, verify that BGPPeer resources
exist on the POP cluster:

```bash
kubectl get bgppeers
```

Check whether sessions have established:

```bash
kubectl get bgppeers -o wide
```

The per-provider `SessionEstablished` condition shows whether the BGP FSM
has reached Established state. If a session is not establishing:

1. Verify network reachability between the node and the peer address
2. Check that FRR/GoBGP is listening on the expected port
3. Inspect the cosmos controller logs:
   ```bash
   kubectl -n bgp-system logs -l app.kubernetes.io/name=cosmos
   ```
4. Check the per-provider conditions on the BGPPeer:
   ```bash
   kubectl describe bgppeer <peer-name>
   ```

---

## Step 8: Advertise infrastructure prefixes (underlay only)

For loopback addresses and SRv6 locator blocks, create a BGPAdvertisement
on the POP cluster:

```yaml
apiVersion: bgp.miloapis.com/v1alpha1
kind: BGPAdvertisement
metadata:
  name: node-1-loopback
spec:
  instanceRef: underlay
  prefixes:
    - "2001:db8:loopback::1/128"
```

Apply:

```bash
kubectl apply -f advertisement.yaml
```

Verify the advertisement:

```bash
kubectl get bgpadvertisement node-1-loopback -o jsonpath='{.status.providers}'
```

The `Advertised: True` condition in the per-provider status confirms the prefix
is in the FRR RIB.

---

## Next steps

- Read the full [API reference](api/) for all 6 CRDs, including field
  definitions, conditions, and the operational contract between cosmos and the
  CNI plugin.
- Review the [example files](examples/) for complete annotated YAML:
  - [BGPProvider (auto-bootstrapped)](examples/pop-bgpprovider-auto.yaml)
  - [BGPInstance — underlay FRR](examples/pop-bgpinstance-underlay.yaml)
  - [BGPInstance — overlay GoBGP](examples/pop-bgpinstance-overlay.yaml)
  - [BGPInstance — infra route reflector](examples/infra-bgpinstance-rr.yaml)
  - [BGPExternalPeer — ToR switch](examples/mgmt-bgpexternalpeer-tor.yaml)
  - [Rejected configurations](examples/rejected-configurations.yaml)

<!-- References -->
[bgp]: https://datatracker.ietf.org/doc/html/rfc4271
[gobgp]: https://github.com/osrg/gobgp
[frr]: https://frrouting.org/
