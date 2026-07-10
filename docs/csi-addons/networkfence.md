# Network Fencing

Network fencing is a [CSI-Addons](https://csi-addons.github.io/) feature that
provides the ability to block and unblock network access to storage resources
based on CIDR blocks. This capability is crucial for disaster recovery scenarios,
particularly for handling non-graceful node shutdowns and preventing split-brain
situations in storage clusters.

## Overview

Network fencing in Ceph-CSI leverages Ceph's OSD blocklist functionality to
deny access to specific IP addresses or IP ranges. When a node becomes
unresponsive or needs to be isolated, network fencing ensures that the node
cannot access storage resources, preventing potential data corruption.

The network fence feature is implemented through the CSI-Addons specification
and supports both RBD (RADOS Block Device) and CephFS storage types.

## How It Works

Network fencing operates by:

1. **Blocking Access**: Adding IP addresses or CIDR blocks to the Ceph OSD
   blocklist, which prevents clients from those addresses from accessing the
   storage cluster.

1. **Unblocking Access**: Removing IP addresses or CIDR blocks from the Ceph
   OSD blocklist to restore access.

1. **Client Eviction** (CephFS): For CephFS volumes, active client sessions
   are evicted as an additional step before adding the blocklist entry to
   ensure immediate disconnection. Blocklisting itself is used for both CephFS
   and RBD volumes.

1. **Auto-Unfencing**: Automatically removing blocklist entries when certain
   conditions are met, such as when a node recovers and needs to reconnect.

## Key Features

### Range-Based Blocklisting

Ceph-CSI supports range-based blocklisting when the Ceph cluster version
supports it. This allows blocking entire CIDR ranges with a single command,
improving performance for large IP ranges.

If range-based blocklisting is not supported, Ceph-CSI automatically falls
back to blocking individual IPs within the CIDR range.

### CephFS Client Eviction

For CephFS volumes, network fencing includes an additional step of evicting
active client sessions. This CephFS-specific step complements the OSD
blocklisting that is used for both CephFS and RBD volumes. It ensures that
clients are immediately disconnected from the filesystem before being
blocklisted, preventing any lingering connections.

The eviction process:

1. Lists all active CephFS clients
1. Identifies clients whose IP addresses fall within the specified CIDR blocks
1. Evicts matching clients using the MDS (Metadata Server)
1. Adds the CIDR blocks to the OSD blocklist

### Auto-Unfencing

Auto-unfencing is a feature that automatically removes blocklist entries when
a node recovers. This is particularly useful in scenarios where:

- A node was temporarily unavailable but has now recovered
- The blocklist entry was created with a specific expiration time
- The node needs to reconnect to storage without manual intervention

Auto-unfencing occurs when:

1. The time remaining until blocklist expiry is less than or equal to the
   configured auto-blocklist time
1. A cool-down period (5 minutes by default) has passed since the blocklist
   was created
1. The client attempts to reconnect via the `GetFenceClients` operation

## CSI-Addons Operations

### FenceClusterNetwork

Blocks access to storage resources for all IP addresses within the specified
CIDR blocks.

**Request Parameters:**

- `cidrs`: List of CIDR blocks to fence (e.g., "192.168.1.0/24")
- `parameters`: Configuration options (clusterID is required)
- `secrets`: Ceph credentials for authentication

**Example:**

```yaml
apiVersion: csiaddons.openshift.io/v1alpha1
kind: NetworkFence
metadata:
  name: network-fence-example
spec:
  driver: rbd.csi.ceph.com
  fenceState: Fenced
  cidrs:
    - "192.168.1.0/24"
  parameters:
    clusterID: "my-ceph-cluster"
  secret:
    name: csi-rbd-secret
    namespace: default
```

### UnfenceClusterNetwork

Removes the network fence, restoring access to storage resources for the
specified CIDR blocks.

**Request Parameters:**

- `cidrs`: List of CIDR blocks to unfence
- `parameters`: Configuration options (clusterID is required)
- `secrets`: Ceph credentials for authentication

**Example:**

```yaml
apiVersion: csiaddons.openshift.io/v1alpha1
kind: NetworkFence
metadata:
  name: network-fence-example
spec:
  driver: rbd.csi.ceph.com
  fenceState: Unfenced
  cidrs:
    - "192.168.1.0/24"
  parameters:
    clusterID: "my-ceph-cluster"
  secret:
    name: csi-rbd-secret
    namespace: default
```

### GetFenceClients

Retrieves information about clients that need to be fenced, including the
cluster ID and client addresses. This operation also triggers auto-unfencing
if the conditions are met.

**Response:**

- `clients`: List of client details (id: Ceph cluster FSID, addresses: CIDR blocks)

## Use Cases

### Non-Graceful Node Shutdown

When a Kubernetes node fails or becomes unresponsive, network fencing can be
used to:

1. Prevent the failed node from accessing storage
1. Allow volumes to be safely attached to other nodes
1. Ensure data consistency and prevent corruption

This is particularly important for stateful applications that require exclusive
access to storage volumes.

### Disaster Recovery

In disaster recovery scenarios, network fencing helps:

1. Isolate failed or compromised nodes
1. Prevent split-brain situations in clustered applications
1. Enable safe failover to backup sites

### Maintenance Operations

During planned maintenance:

1. Fence nodes before taking them offline
1. Perform maintenance operations safely
1. Unfence nodes after maintenance is complete

## Configuration

### Enabling Network Fencing

The `--enable-fencing` flag in the Ceph-CSI driver configuration enables
auto-fencing. CSI-Addons fencing operations work without this parameter, but
automatic fencing behavior requires it to be set to `true`.

**Example deployment configuration:**

```yaml
args:
  - "--enable-fencing=true"
  - "--csi-addons-endpoint=unix:///tmp/csi-addons.sock"
```

### CSI-Addons Endpoint

The CSI-Addons endpoint must be configured to enable communication between
the CSI-Addons controller and the Ceph-CSI driver:

```yaml
args:
  - "--csi-addons-endpoint=unix:///tmp/csi-addons.sock"
```

## Prerequisites

1. **CSI-Addons Controller**: The CSI-Addons controller must be deployed in
   the Kubernetes cluster. See the [CSI-Addons documentation](https://csi-addons.github.io/)
   for installation instructions.

1. **Network Fence CRDs**: The NetworkFence Custom Resource Definitions must
   be installed:

   ```bash
   kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.10.0/deploy/controller/crds.yaml
   ```

1. **RBAC Permissions**: Appropriate RBAC rules must be configured for the
   CSI-Addons operator to manage network fences.

1. **Ceph Credentials**: Valid Ceph credentials with permissions to manage
   the OSD blocklist.

## Limitations and Considerations

### Cool-Down Period

A 5-minute cool-down period is enforced after creating a blocklist entry
before auto-unfencing can occur. This prevents rapid fence/unfence cycles
that could cause instability.

### IPv4 and IPv6 Support

Network fencing supports both IPv4 and IPv6 addresses. The implementation
automatically detects the IP version and applies the appropriate CIDR suffix:

- IPv4: `/32` for single hosts
- IPv6: `/128` for single hosts

### Ceph Version Compatibility

Range-based blocklisting requires a recent version of Ceph that supports the
`osd blocklist range add` and `osd blocklist range rm` commands. If your Ceph
cluster doesn't support these commands, Ceph-CSI will automatically fall back
to individual IP blocklisting.

### CephFS MDS Rank

For CephFS client eviction, Ceph-CSI uses MDS rank 0 by default, as all
clients maintain a session with rank 0. This is sufficient for most
deployments.

## Troubleshooting

### Checking Blocklist Status

To verify if an IP is blocklisted:

```bash
ceph osd blocklist ls
```

### Manual Blocklist Management

If needed, you can manually manage the blocklist:

```bash
# Add an IP to the blocklist
ceph osd blocklist add 192.168.1.10

# Remove an IP from the blocklist
ceph osd blocklist rm 192.168.1.10

# Add a CIDR range (if supported)
ceph osd blocklist range add 192.168.1.0/24

# Remove a CIDR range (if supported)
ceph osd blocklist range rm 192.168.1.0/24
```

### Common Issues

1. **"invalid command" error**: Your Ceph version doesn't support range-based
   blocklisting. Ceph-CSI will automatically fall back to individual IP
   blocklisting.

1. **Auto-unfencing not working**: Ensure the cool-down period (5 minutes)
   has passed and the blocklist entry's expiration time is within the
   configured auto-blocklist time window. Also verify that the
   `--enable-fencing` parameter is set to `true` in the Ceph-CSI driver
   configuration, as auto-fencing requires this flag to be enabled.

1. **CephFS client eviction fails**: Verify that the MDS is healthy and
   accessible, and that the credentials have sufficient permissions.

## See Also

- [CSI-Addons Documentation](https://csi-addons.github.io/)
- [CSI-Addons Specification](https://github.com/csi-addons/spec)
- [Ceph OSD Blocklist Documentation](https://docs.ceph.com/en/latest/rados/operations/control/#blocklisting)
- [Non-Graceful Node Shutdown Design](../design/proposals/non-graceful-node-shutdown.md)
