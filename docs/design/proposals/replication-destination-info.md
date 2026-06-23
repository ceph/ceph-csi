# GetReplicationDestinationInfo Implementation

**Status**: Proposal
**Version**: 1.0
**Date**: 2026-06-03

## Summary

Implement the `GetReplicationDestinationInfo` RPC in ceph-csi to provide
destination volume and volume group identifiers for disaster recovery
orchestration. This enables DR orchestrators to discover the correct
destination volume IDs and group IDs when pools have different IDs across
mirrored clusters.

## Motivation

### Problem Statement

During disaster recovery, RBD volumes are mirrored from a primary cluster to
a secondary cluster. The CSI volume ID encoding includes:

- Cluster ID (e.g., "primary-cluster", "secondary-cluster")
- Pool ID (e.g., pool ID 1 on primary, pool ID 5 on secondary)
- Object UUID (same on both clusters due to mirroring)

**Example:**

Primary cluster volume ID:

```text
0001-000f-primary-cluster-0000000000000001-vol-abc123
         └─────┬──────┘   └──────┬──────┘ └────┬───┘
           clusterID        poolID=1         uuid
```

Secondary cluster volume ID (after mirroring):

```text
0001-0011-secondary-cluster-0000000000000005-vol-abc123
         └───────┬───────┘   └──────┬──────┘ └────┬───┘
           clusterID          poolID=5         uuid (same)
```

**Challenge**: The DR orchestrator needs to know the destination volume ID to
create PVs during failover, but currently has no standard way to obtain this
information.

### Goals

- Implement `GetReplicationDestinationInfo` RPC for RBD volumes
- Implement `GetReplicationDestinationInfo` RPC for RBD volume groups
- Support pool name-based mapping (intuitive, pool names are consistent
  across clusters)
- Maintain backward compatibility with existing cluster-mapping.json

### Non-Goals

- Multi-destination support per cluster

### Key Requirement: ClientProfileMapping Integration

The RPC **must** consider ClientProfileMapping when resolving cluster IDs.
This handles the scenario where:

- Old PVs reference destroyed clusters
- ClientProfileMapping maps old cluster IDs to current cluster
- GetReplicationDestinationInfo needs to work for these old PVs

**Flow:**

```text
volumeHandle contains: cluster-a
  ↓
Check ClientProfileMapping: cluster-a → cluster-d
  ↓
Get ReplicationDestinationConfig for cluster-d
  ↓
Return destination info
```

Over time, as users failover and failback, old PVs will naturally be replaced
with new ones, making ClientProfileMapping obsolete (but it can remain
indefinitely).

## Proposal

### Capability Advertisement

The CSI driver **must** advertise the `GET_REPLICATION_DESTINATION_INFO`
capability via the `GetCapabilities` RPC. This allows DR orchestrators to
discover whether the driver supports this feature.

```go
identity.Capability_VolumeReplication_GET_REPLICATION_DESTINATION_INFO
```

### Architecture

```
┌─────────────────────────────────────────────┐
│  1. ConfigMap (csi-config)                  │
│     replicationDestination field            │
│     - Single destination per cluster        │
│     - Pool name → remote pool ID mapping    │
└─────────────────┬───────────────────────────┘
                  │ CSI reads at startup
                  ↓
┌─────────────────────────────────────────────┐
│  2. CSI Implementation                      │
│     GetReplicationDestinationInfo RPC       │
│     - Decode volume ID                      │
│     - Resolve pool name                     │
│     - Map to remote pool ID                 │
│     - Construct destination volume ID       │
└─────────────────────────────────────────────┘
```

### Configuration Schema

#### 1. CSI Config Extension

Extend `ClusterInfo` in `api/deploy/kubernetes/csi-config-map.go`:

```go
type ClusterInfo struct {
    ClusterID string   `json:"clusterID"`
    Monitors  []string `json:"monitors"`
    RBD       RBD      `json:"rbd"`
    CephFS    CephFS   `json:"cephFS"`
    NFS       NFS      `json:"nfs"`

    // ReplicationDestination defines the destination cluster for replication
    // Populated by ceph-csi-operator from ReplicationDestinationConfig CR
    // +optional
    ReplicationDestination *ReplicationDestinationInfo `json:"replicationDestination,omitempty"`
}

type ReplicationDestinationInfo struct {
    // RemoteClusterID is the clusterID of the destination cluster
    RemoteClusterID string `json:"remoteClusterID"`

    // RBD contains RBD-specific replication destination configuration
    // +optional
    RBD *RemoteRBDDetails `json:"rbd,omitempty"`
}

type RemoteRBDDetails struct {
    // RemotePoolMapping maps pool names to remote pool details
    // Key: pool name (e.g., "rbd", "replicapool")
    // If empty, pool IDs are assumed identical on both clusters
    // +optional
    RemotePoolMapping map[string]RemotePoolDetails `json:"remotePoolMapping,omitempty"`
}

type RemotePoolDetails struct {
    // PoolID is the remote pool ID
    PoolID string `json:"poolID"`
}
```

#### 2. Example ConfigMap

```json
[{
  "clusterID": "primary-cluster",
  "monitors": ["10.0.0.1:6789"],
  "replicationDestination": {
    "remoteClusterID": "secondary-cluster",
    "rbd": {
      "remotePoolMapping": {
        "rbd": {
          "poolID": "5"
        },
        "replicapool": {
          "poolID": "6"
        }
      }
    }
  }
}]
```

### Key Insight: Reuse GetVolumeByID

`GetVolumeByID` already handles ClientProfileMapping internally!

**We just need:**

- Get pool name from rbdVol (already resolved)
- Look up in ReplicationDestinationConfig
- Build destination volume ID

### Implementation Algorithm

**GetReplicationDestinationInfo RPC - Simplified Algorithm:**

1. **Validate source volume exists and is mirrored**:
  - Call `GetVolumeByID(volumeID)` → returns `rbdVol`
  - **This already handles ClientProfileMapping internally!**
  - `rbdVol` now has actual cluster ID and pool name (not original from
    volumeHandle)
  - Verify mirroring is enabled on the volume
1. **Extract resolved identifiers** from `rbdVol`:
  - Get actual cluster ID: `rbdVol.GetClusterID()` (after
    ClientProfileMapping)
  - Get actual pool name: `rbdVol.GetPool()` (after ClientProfileMapping)
  - Get object UUID: decode from `rbdVol.VolID`
1. **Get ReplicationDestinationConfig** for actual cluster ID:
  - Read `replicationDestination` from csi-config ConfigMap
  - If not configured, assume same volumeHandle on remote
1. **Map pool name to remote pool ID**:
  - Look up pool name in `ReplicationDestination.RemotePoolMapping`
  - Get remote pool ID (e.g., "rbd" → "5")
  - If no mapping exists, assume same pool ID on remote
1. **Construct destination volume ID**:
  - Combine: remoteClusterID + remotePoolID + objectUUID
  - Encode as CSI volume ID
1. **Return destination volume ID** to DR orchestrator

#### Detailed Example Walkthrough (Simplified)

Scenario: Old PV from destroyed cluster-a

Input volumeHandle: `0001-000f-cluster-a-0000000000000001-vol-abc123`

Step 1: Validate volume (GetVolumeByID handles ClientProfileMapping
automatically)

```go
rbdVol, err := mgr.GetVolumeByID(ctx, volumeID)
// GetVolumeByID internally:
// - Decodes volumeID: cluster-a, poolID=1
// - Finds cluster-a not in csi-config
// - Checks ClientProfileMapping: cluster-a → cluster-d, poolID 1 → poolID 3
// - Connects to cluster-d
// - Loads volume from cluster-d, pool ID 3
// - Returns rbdVol with actual cluster/pool info
```

Step 2: Extract resolved information

```go
actualClusterID := rbdVol.GetClusterID()  // "cluster-d" (not cluster-a!)
poolName := rbdVol.GetPool()          // "rbd"
objectUUID := rbdVol.GetID()              // from volumeID
```

Step 3: Get ReplicationDestination

```go
destInfo := util.GetReplicationDestinationInfo("cluster-d")
// Returns:
// remoteClusterID: "cluster-b"
// RemotePoolMapping: {"rbd": {"poolID": "5"}}
```

Step 4: Map pool name to remote pool ID

```go
poolDetails := destInfo.RBD.RemotePoolMapping["rbd"]
remotePoolID := "5"
```

Step 5: Construct destination ID

```go
destID := ComposeCSIID("cluster-b", "5", "vol-abc123")
// Result: 0001-0011-cluster-b-0000000000000005-vol-abc123
```

Step 6: Return

```text
0001-0011-cluster-b-0000000000000005-vol-abc123
```

### Volume Groups Support

Volume group IDs have **identical structure** to volume IDs:

```
Volume ID:       clusterID + poolID + volumeUUID
Volume Group ID: clusterID + poolID + groupUUID
```

**Therefore**: Use the same algorithm function for both!

```go
func (rs *ReplicationServer) GetVolumeGroupReplicationDestinationInfo(
    ctx context.Context,
    req *replication.GetReplicationDestinationInfoRequest,
) (*replication.GetReplicationDestinationInfoResponse, error) {
    // ... validation and locking ...

    // Map group ID (same function as volumes!)
    destinationGroupID, err := getDestinationIDFromReplicationConfig(
        ctx, volumeGroupID, clusterID, cr)

    // Map each volume in the group
    volumeIDMappings := make(map[string]string)
    for _, vol := range volumes {
        sourceVolID, _ := vol.GetID(ctx)
        destVolID, _ := getDestinationIDFromReplicationConfig(
            ctx, sourceVolID, clusterID, cr)
        volumeIDMappings[sourceVolID] = destVolID
    }

    return &replication.GetReplicationDestinationInfoResponse{
        ReplicationDestination: &replication.ReplicationDestination{
            Destination: &replication.ReplicationDestination_VolumeGroupDestination{
                VolumeGroupDestination: &replication.VolumeGroupDestination{
                    VolumeGroupId: destinationGroupID,
                    VolumeIds:     volumeIDMappings,
                },
            },
        },
    }, nil
}
```

## References

- Upstream design:
  <https://github.com/csi-addons/kubernetes-csi-addons/pull/993>
- CSI-Addons spec: <https://github.com/csi-addons/spec/pull/79>
- Existing cluster-mapping.json: `docs/design/proposals/clusterid-mapping.md`
