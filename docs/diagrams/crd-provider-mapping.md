# CRD to BGPProvider Struct Mapping

This diagram shows how Kubernetes CRDs flow through reconcilers, get converted to provider structs, and are dispatched to the `Provider` interface.

```mermaid
flowchart TD
    subgraph CRDs["Kubernetes CRDs (bgp.miloapis.com/v1alpha1)"]
        BGPProvider["BGPProvider\n(providers.miloapis.com)\n─────────────────\nspec.type: FRR|GoBGP\nspec.endpoint"]
        BGPInstance["BGPInstance\n─────────────────\nspec.asNumber\nspec.routerID\nspec.listenPort\nspec.addressFamilies[]\nspec.timers\nspec.bestPath\nspec.routeReflector"]
        BGPPeer["BGPPeer\n─────────────────\nspec.address\nspec.asNumber\nspec.addressFamilies[]\nspec.timers\nspec.allowAsIn\nspec.routeReflectorClient\nspec.passive\nspec.ebgpMultihop\nspec.ttlSecurity\nspec.remotePort\nspec.passwordRef"]
        BGPAdvertisement["BGPAdvertisement\n─────────────────\nspec.prefixes[]\nspec.peerSelector"]
        BGPRoutePolicy["BGPRoutePolicy\n─────────────────\nspec.priority\nspec.importStatements[]\nspec.exportStatements[]"]
        BGPSession["BGPSession\n─────────────────\nspec.localPeerRef\nspec.externalPeerRef"]
        BGPExternalPeer["BGPExternalPeer\n─────────────────\nspec.address\nspec.asNumber"]
    end

    subgraph Reconcilers["Controllers (internal/controller)"]
        ProviderRec["ProviderReconciler\nprovider_reconciler.go"]
        InstanceRec["InstanceReconciler\ninstance_reconciler.go"]
        PeerRec["PeerReconciler\npeer_reconciler.go"]
        AdvRec["AdvertisementReconciler\nadvertisement_reconciler.go"]
        PolicyRec["RoutePolicyReconciler\nroutepolicy_reconciler.go"]
        SessionRec["SessionReconciler\nsession_reconciler.go"]
    end

    subgraph ProviderStructs["Provider Structs (internal/provider/provider.go)"]
        InstanceSpec["InstanceSpec\n─────────────────\nASNumber uint32\nRouterID string\nListenPort int32\nFamilies []AddressFamily\nTimers\nBestPath\nRouteReflector"]
        PeerSpec["PeerSpec\n─────────────────\nAddress string\nASNumber uint32\nFamilies []AddressFamily\nTimers\nAllowAsIn uint32\nRouteReflectorClient bool\nPassive bool\nEBGPMultihop uint32\nTTLSecurity uint32\nPassword string\nRemotePort uint32"]
        AdvSpec["AdvertisementSpec\n─────────────────\nPrefixes []string\nPeerAddresses []string"]
        PolicySpec["PolicySpec\n─────────────────\nName string\nPriority int32\nImportStatements []\nExportStatements []"]
        PolicyStatement["PolicyStatement\n─────────────────\nName string\nConditions PolicyConditions\nActions PolicyActions"]
    end

    subgraph Interface["Provider Interface (internal/provider/provider.go:41)"]
        PI["provider.Provider\n────────────────────────────────\nConfigureInstance(InstanceSpec)\nAddOrUpdatePeer(PeerSpec)\nDeletePeer(address string)\nAddOrUpdateAdvertisement(AdvSpec)\nDeleteAdvertisement(prefix string)\nAddOrUpdatePolicy(PolicySpec)\nDeletePolicy(policyName string)\nReady()\nCapabilities()"]
    end

    subgraph Implementations["Provider Implementations"]
        GoBGP["GoBGP\ninternal/provider/gobgp/"]
        FRR["FRR\ninternal/provider/frr/"]
    end

    subgraph Registry["Provider Registry"]
        Reg["provider.Registry\nmap: name → Provider"]
    end

    %% CRD → Reconciler
    BGPProvider --> ProviderRec
    BGPInstance --> InstanceRec
    BGPPeer --> PeerRec
    BGPAdvertisement --> AdvRec
    BGPRoutePolicy --> PolicyRec
    BGPSession --> SessionRec
    BGPExternalPeer --> SessionRec

    %% Session generates BGPPeer (no direct provider call)
    SessionRec -. "generates BGPPeer\n+ BGPSession CRDs" .-> BGPPeer

    %% Reconciler → Provider Struct conversion
    InstanceRec --> InstanceSpec
    PeerRec --> PeerSpec
    AdvRec --> AdvSpec
    PolicyRec --> PolicySpec
    PolicySpec --> PolicyStatement

    %% Provider Struct → Interface Method
    InstanceSpec --> |"ConfigureInstance()"| PI
    PeerSpec --> |"AddOrUpdatePeer()\nDeletePeer()"| PI
    AdvSpec --> |"AddOrUpdateAdvertisement()\nDeleteAdvertisement()"| PI
    PolicySpec --> |"AddOrUpdatePolicy()\nDeletePolicy()"| PI

    %% Provider registration
    ProviderRec --> |"factory(type, endpoint)\nRegistry.Set()"| Reg
    Reg --> |"Registry.Get()"| PI

    %% Interface → Implementation
    PI --> GoBGP
    PI --> FRR
```

## Key Reference Resolution

Before building provider structs, reconcilers resolve indirect references:

| Reconciler | Resolution Step |
|---|---|
| **InstanceReconciler** | RouterID: resolves from node annotation or Downward API env `NAMESPACE` |
| **PeerReconciler** | Timers: merges instance-level defaults with peer-level overrides |
| **PeerReconciler** | Password: fetches from `spec.passwordRef` Secret |
| **AdvertisementReconciler** | PeerAddresses: expands `spec.peerSelector` label query → `[]string` of peer IPs |
| **ProviderReconciler** | Endpoint: extracted from `BGPProvider.Spec`, passed to `ProviderFactory` |

## CRD Ownership Hierarchy

```
BGPProvider  ←── BGPInstance (providerSelector)
                      ↑
              BGPPeer (instanceRef + providerRef/Selector)
              BGPAdvertisement (instanceRef)
              BGPRoutePolicy (instanceRef)
                      ↑
              BGPSession → generates BGPPeer + links BGPExternalPeer
```

## Files

| Component | Path |
|---|---|
| BGP CRD types | `api/bgp/v1alpha1/*_types.go` |
| Provider CRD type | `api/providers/v1alpha1/provider_types.go` |
| Provider interface + structs | `internal/provider/provider.go` |
| Reconcilers | `internal/controller/*_reconciler.go` |
| GoBGP implementation | `internal/provider/gobgp/` |
| FRR implementation | `internal/provider/frr/` |
