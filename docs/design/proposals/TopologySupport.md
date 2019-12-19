# Topology aware provisioning support for Ceph-CSI

This document details the design around adding topology aware provisioning support to ceph CSI drivers.

## Definitions

**NOTE:** Used from kubernetes "[Volume Topology-aware Scheduling](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/volume-topology-scheduling.md#definitions)"

  - Topology: Rules to describe accessibility of an object with respect to location in a cluster.
  - Domain: A grouping of locations within a cluster. For example, 'node1', 'rack10', 'zone5'.
  - Topology Key: A description of a general class of domains. For example, 'node', 'rack', 'zone'.
  - Hierarchical domain: Domain that can be fully encompassed in a larger domain. For example, the 'zone1' domain can be fully encompassed in the 'region1' domain.
  - Failover domain: A domain that a workload intends to run in at a later time.

## Use case

The need to add topology support in Ceph CSI drivers, comes from the following use-case,

Data access costs (performance and network) are uneven across OSDs, based on where the primary OSDs are located for each placement group (PG) in the crush map for the pool(s) backing the volumes
  - The read problem for cross-domain OSDs
    - Assuming different PGs have different OSDs as their primary OSD, data reads are served from these primary OSDs
    - This creates access to different OSDs, from the client, which maybe not be co-located in the same domain as the client
    - Which in turn can cause,
      - Increased network latency, impacting performance, assuming the primary OSD is further away or in a different domain than the client and there exists an OSD within the same domain as the client
      - Network bandwidth costs in cloud or other environments, where such cross domain access is charged based on usage
  - The write problem for cross-domain OSDs
    - Writes to OSDs need to cross domain boundaries, for replication/disperse requirements, and hence is a cost that is unavoidable and required to retain cross domain availability of data and accessibility of the volume
    - To optimize the write costs, OSDs backing a pool can be selected from a single domain
      - But may remain unavailable if the domain is not accessible, which is a trade-off if writes also need to be optimized

To resolve issues such as the one above, we need to introduce topology aware provisioning support, to dynamically provision volumes using pools, whose primary OSDs across PGs, or all OSDs in the PG, are setup to be co-located within a single domain. This enables a transparent choice for applications that are consuming persistent storage from ceph, instead of requesting storage from a particular domain (say via named StorageClasses in kubernetes).

**NOTE:** Creation of pools with all primary OSDs in all PGs co-located within a single domain, or all OSDs co-located within a single domain, is not in scope of this document

## Topology support in CSI specification

CSI nodeplugins report topology labels that "Specifies where (regions, zones, racks, etc.) the node is accessible from." in response to the nodeplugin NodeGetInfo RPC [1]. COs would typically use this information, along with topology information returned via the CreateVolumeResponse, to schedule workloads on nodes that support the specific topology that the volume was created to be accessed from.

CSI controller plugins CreateVolume RPC receives create requests with a `TopologyRequirement` [2] that specifies where the volume must be accessible from. The response to the CreateVolume RPC in turn contains the `Topology` from where the provisioned volume is accessible. The returned `Topology` should adhere to the requested `TopologyRequirement` such that at least some parts of the requirement are satisfied.

**NOTE:** (for completeness) The CSI identity service (part of both the controller and node plugins) declares topology support by the CSI driver via the GetPluginCapabilities RPC [3].

The above pretty much sums up topology related information provided in the CSI spec as such. The questions around implementation that arise are,

#### Where does the CO get required information fill up TopologyRequirement in a CreateVolume request?

The COs determine supported topologies by the CSI driver, by forming a set of domain labels that all node plugins of the said driver advertise. This is used as needed in a CreateVolume request, all the way from restricting the volume to be accessible from a specific topology, or sent as a whole for the CSI controller plugin to decide which topologies the created volume can be accessible from.

#### How do nodeplugins decide which topology they support or even belong to, such that the same can be advertised?

This is slightly more convoluted, as nodeplugins need knowledge of which node they are running on, and what is the domain definition for that node. Both pieces of information, nodeid and domain of the node, are passed back to the COs via the NodeGetInfo RPC.

#### How does the CSI plugin choose where to allocate the volume from?

Given a `CreateVolume` request with topology constraints, the CSI controller plugin needs to decide where to provision the volume from. Typically cloud providers add the constraint to the request that is further made to their management API servers [11]. Also as the nodes are running within the said cloud, the node domain is set to reflect what is know by the cloud providers. In the case of Ceph-CSI, the plugins need to be aware of domain affinity of various pools, to make an educated selection on which pool to create the volume on.

## Topology aware volume implementation in kubernetes

**NOTE:** Kubernetes CSI developer guide provides this [4] section for topology support and how it is built into kubernetes CSI. Elements from the same are not detailed in this section.

**NOTE:** There is also a design document that details kubernetes topology aware volume implementation that can be found here [12]

The following discussion is around how kubernetes passes around topology requirements and stores the same, and as a result, requirements for CSI plugins to be aware of.

#### Kubernetes StorageClass parameter `volumeBindingMode`[5]:
  
Kubernetes StorageClass defines the parameter `volumeBindingMode` that supports values of `Immediate` and `WaitForFirstConsumer`.

  - When `volumeBindingMode` is `WaitForFirstConsumer`, the provision request is made to the CSI controller plugin post the kubernetes scheduler decides on which node to schedule the Pod on. Such `CreateVolume` requests will come in with a `TopologyRequirement` that contains the domain as advertized by the corresponding nodeplugin, listed ahead of all other domains supported by all nodeplugins in the `preferred` section **[reference needed]**.
  - When `volumeBindingMode` is `Immediate`, the CreateVolume request is sent a `TopologyRequirement` that is a union of all comains as advertized by all nodeplugins

Due to the above, the values in `TopologyRequirement` may range from a singleton to all supported domains, and thus, the distribution of volumes evenly across pools that support the said domains becomes a requirement of the CSI plugin, when the `volumeBindingMode` is `Immediate`.

#### Kubernetes StorageClass parameter `allowedTopologies`[6]:

As per the kubernetes documentation "When a cluster operator specifies the WaitForFirstConsumer volume binding mode, it is no longer necessary to restrict provisioning to specific topologies in most situations. However, if still required, allowedTopologies can be specified."

If `allowedTopologies` is specified then further restrictions on primary domain for the provisioned volume needs to be applied by the CSI provisioner.

**NOTE:** For the current implementation, `allowedTopologies` is not planned to be supported.

#### Topology details stored by kubernetes on the PV [9]

Kubernetes stores the volume topology that is passed back as a response to a successful `VolumeCreate` request in the PV. As PVs are immutable post creation, this ties the PV to topologies that were sent in the response.

Further, the topology constraint that is stored with the PV only contains a `Required` section, thus any node satisfying the topology returned by `CreateVolume` request can be chosen to schedule the pod on. This requires the CSI driver to respond with a singleton `Required` domain value, even when the volume can be accessed across domains, albeit unevenly, to restrict the scheduling of the pod to the required domain.

Thus, when a domain fails, PVs that are tied to that domain cannot be scheduled on other nodes, unless there is a takeover of the failed domain by other nodes running in other domains.

**NOTE:** `Preferred` topology field is a future concern as of now [9], and may possibly come with weights that can help skewing the domain preference, while returning all topologies that the volume can be accessed from in the response to a `CreateVolume` request.

## Ceph-CSI topology support design

Supporting topology aware provisioning in Ceph-CSI reduces to solving the following problems as a result of the above discussion,

### Determining node domain by the CSI nodeplugin

Currently the Ceph-CSI nodeplugins have no information regarding the domains they belong to. The `node_id`, in `NodeGetInfo` response, itself is picked up from the pod manifest [7]. Kubernetes maintains failure-domains[8], but these cannot be passed in via the pod manifest as yet [13], as these are not supported by the downward APIs[14] (unlike the `node-id` for example).

The CSI nodeplugins also do not have any specific domain knowledge to present themselves, and would hence rely on the domain information of the cluster that it runs on.

The proposal is to, feed CSI nodeplugins a list of labels that it can read from the COs to determine its domain and return the same in it's response to a `NodeGetInfo` request.

For example in the case of the CO being kubernetes the following maybe be passed as an ordered list of domain labels to advertise. The order of labels ensures hierarchical domain relationships. The nodeplugin would read the values of these labels, via kubernetes client API, on the node where it is running and respond with the same in the `NodeGetInfo` request.

```
# ceph csi ... --orchestrator="kubernetes" --domain-labels="failure-domain.beta.kubernetes.io/region;failure-domain.beta.kubernetes.io/zone;failure-domain.mycluster.io/rack" ...
```

**NOTE:** Current proposal is to support only kubernetes as the orchestrator

### StorageClass changes to denote pools with domain affinity

As noted earlier, creation of pools that have a domain affinity, is out of scope for the CSI plugins. However, when presented with a set of pools that have the said property, a `CreateVolume` request with `accessibility_requirements` specified, needs to choose the right pool to create the image in (in the case of RBD), or to redirect file's data to (in the case of CephFS).

The proposal is to, add the specified pools and their domain affinity in the StorageClass (or related constructs in non-kubernetes COs), for the controller to choose from. The changes to the StorageClass is detailed in this issue [15].

Further, the CSI journal that is maintained as an OMap within a **single** pool needs to continue even when the volumes are allocated from different pools. As, we need a single pool that can hold information about the various CSI volumes to maintain idempotent responses to various CSI RPCs.

The proposal hence is to also include a `pool` parameter in the StorageClass for RBD based volumes, where the CSI OMap journal would be maintained. This can be one of the subset of pools in the list of domain affinity based pools, or a more highly available cross-domain pool.

**NOTE:** CephFS does not need a special `pool` parameter as we store the CSI journal on the metadata pool backing the `fsname` that is passed, which is a singleton. Hence, the same will continue to be leveraged, even when ceohfs data needs to be redirected to different pools.

#### Limitations

- If a new domain is added to the cluster, then the StorageClass must be changed to provide an additional pool that has an affinity to that domain. By default the CSI plugin would otherwise choose to treat the provisioning as if `Immediate` was specified as the `volumeBindingMode`

#### Alternatives

- Use pool labels (if pools can carry user provided labels in Ceph), to filter pools based on labels and their values. Makes adding a pool for a newer domain easier than meddling with the StorageClass, and also keeps the StorageClass leaner.

### Balancing volume allocation when `TopologyRequirement`, includes more than a single domain

As elaborated in "Kubernetes StorageClass parameter `volumeBindingMode`", both `Immediate` and `WaitForFirstConsumer` `volumeBindingMode`s can request multiple domains in the `TopologyRequirement`. Thus, the CSI plugin has to make a choice on which domain to provision the volume from when the binding requested is `Immediate`.

**NOTE:** For `volumeBindingMode` `WaitForFirstConsumer` the first domain specified in the `preferred` list of topologies would be chosen, as this is the domain where the pod would be scheduled.

The proposal is to, choose a domain at random and provision the volume on the pool the domain belongs to. Further, return an empty `accessible_topology` in the response, as the volume can be accessed from any domain.

#### Alternatives

- We could be smart here in the future based on available space, or density and other such parameters that is periodically read from the Ceph backend and do a more fair allocation across pools.

### Domain takeover and giveback

As, kubernetes PVs are immutable and as discussed in "Topology details stored by kubernetes on the PV [9]" the returned topology is stored in the PV, when a domain becomes unavailable it would be required to maintain availability of the volumes.

The thought around addressing this is, to dynamically inform surviving nodeplugins in other domains to take over the failed domain, as Ceph pools are essentially cross domain accessible. Further, in the event of the failed domain becoming available again, dynamically inform the nodeplugins to giveback the domain.

The proposal is to add a domain takeover map, that passed in via the CSI config map [10]. Each entry in the map would consist of a domain label and the target domain label that should takeover the same. The nodeplugins, in addition to advertizing their domains as read from the domain labels of the running node, will also advertize the domains that they need to takeover.

When a failed domain recovers, the corresponding entry in the domain takeover map is deleted, resulting in a dynamic update to all nodeplugins regarding the giveback of the said domain.

As an example the CSI kubernetes config map is updated as follows for a domain takeover,
```
{
    "version": 2,
    "topology-takeover": {
        "<source-domain-1>": [
            "<target-domain-1>",
            "<target-domain-2>"
        ],
        "<source-domain-2>": [
            "all"
        ],
        "<source-domain-3>": [
            "<target-domain-2>"
        ],
        ...
    },
    "clusters": [
        {"clusterID1": "<cluster-id>", ...},
        ...
    ]
}
```

**NOTE:** topology of a node is common across all ceph clusters that the nodeplugin instance may interact with

#### Limitations

- Need to understand frequency of `NodeGetInfo` calls, such that the CO is updated regarding supported domains by the various nodeplugins on a dynamic takeover or giveback action
- As of now, kubernetes calls `NodeGetInfo` once and has no mechanism of updating this information without a plugin restart

### Snapshots and clone considerations

- **TODO**

## Rook implications

The following are high level assumptions on features that Rook would automate ,
- Feeding of domain labels to read from kubernetes, to CSI nodeplugins
- Creating required pools that have OSDs with required domain affinities
- Automatic domain takeover and giveback handling when a domain is not available and vice verse

## References

[1] [CSI spec NodeGetInfo RPC](https://github.com/container-storage-interface/spec/blob/master/spec.md#nodegetinfo)

[2] [CSI spec CreateVolume RPC](https://github.com/container-storage-interface/spec/blob/master/spec.md#createvolume)

[3] [CSI spec GetPluginCapabilities RPC](https://github.com/container-storage-interface/spec/blob/master/spec.md#getplugincapabilities)

[4] [Kubernetes CSI topology details for CSI implementors](https://kubernetes-csi.github.io/docs/topology.html)

[5] [Kubernetes StorageClass `volumeBindingMode` parameter](https://kubernetes.io/docs/concepts/storage/storage-classes/#volume-binding-mode)

[6] [Kubernetes StorageClass `allowedTopologies` parameter](https://kubernetes.io/docs/concepts/storage/storage-classes/#allowed-topologies)

[7] [Pod manifest feeding node ID to CSI](https://github.com/ceph/ceph-csi/blob/2c9d7114638d3bac043781e300cbb7fdcecff3db/deploy/rbd/kubernetes/v1.14%2B/csi-rbdplugin.yaml#L71-L74)

[8] [Kubernetes failure-domains](https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/#failure-domainbetakubernetesiozone)

[9] Kubernetes topology labels on the PV:

- [Kubernetes topology design: volume topology specification](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/volume-topology-scheduling.md#volume-topology-specification)
- [Kubernetes PV topology information](https://github.com/kubernetes/kubernetes/blob/fcc35b046860ab03851b53ff34a10f6ee0cdecf9/pkg/apis/core/types.go#L308-L311)

[10] [Ceph-CSI config map reference](https://github.com/ceph/ceph-csi/blob/master/examples/csi-config-map-sample.yaml)

[11] Cloud providers forwarding topology requirements:

- [AWS CSI topology handling](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/7255ebd9d57120729011ffc0c059af6b867e5b13/pkg/driver/controller.go#L149-L180)
- **TODO Azure/GCE link?**

[12] [Kubernetes Volume topology-aware design](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/volume-topology-scheduling.md)

[13] [Lack of node labels access via the downward APIs](https://github.com/kubernetes/kubernetes/issues/40610)

[14] [Kubernetes downward API](https://kubernetes.io/docs/tasks/inject-data-application/downward-api-volume-expose-pod-information/)

[15] [Multiple pool storage classes informed by topologyKeys](https://github.com/ceph/ceph-csi/issues/559)
