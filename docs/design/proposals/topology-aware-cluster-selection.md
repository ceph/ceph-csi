# Topology-Aware Multi-Cluster Volume Provisioning

Currently Ceph-CSI supports only a single Ceph cluster per StorageClass. The
`clusterID` parameter in the StorageClass is mandatory and points to exactly one
cluster entry in `config.json`. This works well for single-cluster environments,
but creates a significant limitation for distributed Kubernetes deployments
spanning multiple geographic zones, each backed by a separate Ceph cluster.

In such deployments administrators must create a separate StorageClass per
zone/cluster, and application teams must manually select the correct
StorageClass depending on where their workloads run. This defeats the purpose of
Kubernetes topology-aware scheduling and creates operational overhead.

Reference: https://github.com/ceph/ceph-csi/issues/5177

## Problem

Consider a Kubernetes cluster with nodes spread across two zones, each served
by a separate Ceph cluster:

- `zone-poland` with Ceph cluster `cluster-poland` (monitors: `10.0.1.1:6789`)
- `zone-france` with Ceph cluster `cluster-france` (monitors: `10.0.2.1:6789`)

Today, the administrator must create two StorageClasses:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd-poland
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: "cluster-poland"
  pool: replicapool
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd-france
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: "cluster-france"
  pool: replicapool
```

Application teams must then know which StorageClass to use based on where their
pods will be scheduled. If a pod moves to a different zone, the PVC might point
to a remote cluster, losing data locality.

The goal is to have a **single StorageClass** that automatically selects the
correct Ceph cluster based on the node's topology zone.

## Proposed Solution

### Configuration Changes

#### config.json

Each cluster entry in `config.json` gains an optional `topologyDomainLabels`
field that maps Kubernetes topology label keys to their expected values:

```yaml
apiVersion: v1
kind: ConfigMap
data:
  config.json: |-
    [
      {
        "clusterID": "cluster-poland",
        "topologyDomainLabels": {
          "topology.kubernetes.io/zone": "zone-poland"
        },
        "monitors": [
          "10.0.1.1:6789"
        ],
        "rbd": {
          "radosNamespace": ""
        },
        "cephFS": {
          "subvolumeGroup": "csi"
        }
      },
      {
        "clusterID": "cluster-france",
        "topologyDomainLabels": {
          "topology.kubernetes.io/zone": "zone-france"
        },
        "monitors": [
          "10.0.2.1:6789"
        ],
        "rbd": {
          "radosNamespace": ""
        },
        "cephFS": {
          "subvolumeGroup": "csi"
        }
      }
    ]
metadata:
  name: ceph-csi-config
```

Clusters without `topologyDomainLabels` are ignored during topology-based
selection and continue to work exactly as before.

#### StorageClass

A new parameter `clusterIDs` is introduced as a comma-separated list of
candidate cluster IDs. The StorageClass **must** use
`volumeBindingMode: WaitForFirstConsumer` so that Kubernetes provides topology
hints to the CSI driver via `AccessibilityRequirements` in the `CreateVolume`
request.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd-topology
provisioner: rbd.csi.ceph.com
parameters:
  clusterIDs: "cluster-poland,cluster-france"
  pool: replicapool
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: ceph-system
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
```

> **Note:** The existing `clusterID` parameter continues to work as before.
> When `clusterID` is present, it takes priority and the topology-based
> selection is not used. The `clusterIDs` parameter is only consulted when
> `clusterID` is absent.

### How PV Creation Works

Topology-aware cluster selection relies on the Kubernetes topology mechanism
built into the CSI specification. Understanding how topology information flows
from nodes to the `CreateVolume` call is key to understanding the design.

#### Topology Discovery

When the CSI node plugin (DaemonSet) starts on each node, Kubernetes calls
`NodeGetInfo`. The driver reads the node's Kubernetes labels (configured via
the `--domainlabels` flag) and returns them as `AccessibleTopology` segments.
Kubernetes stores this information in the `CSINode` object.

For example, a node with the label `topology.kubernetes.io/zone=zone-poland`
reports:

```json
{
  "accessible_topology": {
    "segments": {
      "topology.kubernetes.io/zone": "zone-poland"
    }
  }
}
```

#### WaitForFirstConsumer Binding

The StorageClass **must** use `volumeBindingMode: WaitForFirstConsumer`. This
tells Kubernetes to delay volume provisioning until a pod consuming the PVC is
scheduled to a specific node. Without this, Kubernetes calls `CreateVolume`
immediately (with `Immediate` binding) and does not know which node the pod
will run on — so no `AccessibilityRequirements` are provided and topology-based
selection cannot work.

#### AccessibilityRequirements: Preferred vs Requisite

When Kubernetes calls `CreateVolume` after scheduling the pod, it includes
`AccessibilityRequirements` with two lists of topologies:

- **Preferred** — an ordered list of topologies where the volume should ideally
  be created. The first entry is the topology of the node where the pod was
  scheduled. This is what we use for data locality — placing storage close to
  compute.

- **Requisite** — a list of all topologies where the volume is allowed to be
  created (hard constraints). This includes all nodes that have capacity to
  serve the volume.

For example, when a pod is scheduled on a node in `zone-poland` in a cluster
that also has nodes in `zone-france`:

```
Preferred: [zone-poland]                  ← the pod's node
Requisite: [zone-poland, zone-france]     ← all eligible zones
```

The driver checks Preferred first (for data locality), and falls back to
Requisite only if no Preferred topology matches any cluster.

#### End-to-End Flow

When a pod is scheduled on a node in `zone-poland` and requests a PVC from the
topology-aware StorageClass, the following happens:

1. Kubernetes sees `volumeBindingMode: WaitForFirstConsumer` and delays
   provisioning until the pod is scheduled to a specific node.

2. Once the pod is bound to a node, Kubernetes calls `CreateVolume` with
   `AccessibilityRequirements` containing the node's topology segments
   (e.g. `topology.kubernetes.io/zone: zone-poland`).

3. The CSI driver first tries to resolve `clusterID` from the StorageClass
   parameters. Since it is not present, the driver falls back to
   topology-based cluster selection.

4. The driver parses the `clusterIDs` parameter to get the list of candidate
   clusters: `["cluster-poland", "cluster-france"]`.

5. For each candidate, the driver reads the `topologyDomainLabels` from
   `config.json` and matches them against the `AccessibilityRequirements`.
   All labels defined in the cluster's `topologyDomainLabels` must be present
   and have matching values in the topology segments.

6. Preferred topologies (from the CO's scheduling preference) are checked
   first. If no match is found, requisite topologies (hard constraints) are
   checked as a fallback.

7. The first matching cluster is selected. In this example, `cluster-poland`
   matches because its `topologyDomainLabels` contain
   `topology.kubernetes.io/zone: zone-poland`, which matches the node's zone.

8. The selected `clusterID` is used to resolve monitors from `config.json`.
   The driver connects to the Ceph cluster in Poland and creates the RBD image
   (or CephFS subvolume) there.

9. The selected `clusterID` is encoded into the `volumeHandle`, so all
   subsequent operations (NodeStage, ExpandVolume, DeleteVolume) resolve the
   correct cluster automatically, without needing topology selection again.

### Multi-Dimensional Topology

The `topologyDomainLabels` field supports multiple labels for multi-dimensional
matching. For example, a cluster can be associated with both a region and a
zone:

```json
{
  "clusterID": "cluster-poland-az1",
  "topologyDomainLabels": {
    "topology.kubernetes.io/region": "europe",
    "topology.kubernetes.io/zone": "poland-az1"
  }
}
```

All labels must match for the cluster to be selected.

## Impact on Existing Operations

The topology-based cluster selection only affects the `CreateVolume` operation.
All other CSI operations are unaffected because the `volumeHandle` already
contains the selected `clusterID`:

- **NodeStageVolume / NodePublishVolume** — the node plugin decodes the
  `clusterID` from the `volumeHandle` and connects to the correct cluster.
  No topology resolution needed.

- **DeleteVolume / ControllerExpandVolume** — the controller decodes the
  `clusterID` from the `volumeHandle`. Same behavior as today.

- **CreateSnapshot** — uses the source volume's `clusterID`.

The provisioner pod (Deployment) must have network access to monitors of all
Ceph clusters listed in `config.json`. This is already the case when multiple
clusters are configured today. The node plugin pods (DaemonSet) also mount the
same `ceph-csi-config` ConfigMap and can connect to any cluster whose volumes
they need to mount.

Connection lifecycle is unchanged — the driver uses the existing connection pool
(`conn_pool.go`) which manages connections by `monitors|user|keyfile`
combination and auto-recycles unused connections.

## Backward Compatibility

- Existing `config.json` entries without `topologyDomainLabels` work unchanged.
  The new field uses `omitempty` in JSON serialization.

- StorageClasses with a single `clusterID` parameter use the existing fast
  path. The topology selection code is never reached.

- The `clusterIDs` parameter is purely additive. No existing parameters or
  validation rules are removed.

- Volumes created with topology-based selection are indistinguishable from
  volumes created with an explicit `clusterID` — the `volumeHandle` format is
  identical.

## Limitations

- `volumeBindingMode: WaitForFirstConsumer` is required when using `clusterIDs`.
  With `Immediate` binding, Kubernetes does not provide
  `AccessibilityRequirements` and the driver cannot determine the target
  topology.

- The pool name must be the same across all candidate clusters (since a single
  `pool` parameter is specified in the StorageClass). If pools have different
  names, the existing `topologyConstrainedPools` mechanism can be combined with
  this feature in a future iteration.

## Future Work

- Make `clusterID` fully optional when `clusterIDs` is provided (currently both
  are accepted, but at least one is required).
- Combine topology-based cluster selection with `topologyConstrainedPools` for
  selecting both cluster and pool based on topology.
- Add E2E tests with a multi-cluster topology setup.
