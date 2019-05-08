# Stateless Ceph-CSI plugin implementation

## Terminology

- CO: Container orchestration system, communicates with CSI plugins using CSI
  RPCs
- NS: Node Service; CSI plugin service per node in the CO, responsible for
  staging volumes for application pod consumption
- CS: Controller Service; CSI plugin service responsible for creation,
  deletion, snapshots, and expanding volumes for use in the CO system

**NOTE:**

- Discussion below may use k8s specific terminology, but should be extensible
  to similar constructs on other container orchestration(CO) systems

## Proposal/problem summary

This document details a proposed solution, among presented alternatives, to
implement a stateless Ceph-CSI plugin. It also details a naming scheme for
volumes provisioned against a Ceph cluster and the resultant *volume_id* for
the same that is shared with the CO system, enabling state to be carried by the
*volume_id* where required.

As of this writing, the plugin uses a config map to store *volume_name* to
*volume_id* mapping, and also stores related Ceph cluster configuration
information (e.g MON list, pool information) within the same tuple, to be able
to find and operate on the provisioned volume. The implementation also depends
on secrets from the StorageClass to perform NS related operations.

The existing strategy enables a single CSI instance to operate across any Ceph
cluster that is fully defined by the StorageClass, but brings in the burden of
storing and managing provisioned volumes by the plugin.

The proposal laid out here is to try and retain the ability for a single CSI
instance to operate across any Ceph cluster, but remove the storage and
management of provisioned CSI volumes by the plugin, or in short make the
Ceph-CSI plugin stateless.

## Proposed solution details

### Summary

- Create a config file for the CSI plugins mapped to `/etc/ceph-csi-config/config.json`
  - **NOTE:** This would get mapped into the plugin container as a config map
  in kubernetes CO environments
- Details about the Ceph cluster are maintained in `config.json`. Details
  include a cluster-id, and MONs. These are updated as new clusters are
  used to provision volumes from, or as MON addresses are changed for
  existing clusters
- VolumeCreate requests come against a targeted Ceph cluster-id and pool or fsname,
  enabling the CSI plugin to locate the appropriate configuration information
  for the cluster to act on the request
  - **NOTE:** The cluster-id and pool/fsname information is specified in the
  StorageClass for example
- The returned *volume_id* from VolumeCreate requests, carries information
  regarding the *cluster_id* and encoded pool/fsname IDs, enabling *volume_id* based
  RPC requests to operate using the provided configuration information
  against the respective cluster
- Usual CSI spec based parameter maps such as, *volume_context* and *publish_context*
  continue to carry information regarding required aspects about the
  provisioned volume to dependent RPCs
- The NS also has access to the `config.json` data to satisfy required node
  level RPC requests

### Pros

- Addresses removal of the current config map that maintains list of
  provisioned volumes
- Provides the ability to have a single Ceph-CSI plugin instance per CO
  instance, even when the CO instance needs volumes provisioned from multiple
  Ceph clusters
- Also, does not take away the capability, where needed, to use multiple
  Ceph-CSI plugin instances (specialized using provisioner name) per CO
  instance

### Cons

- Only RPCs that support passing credentials via secrets to the plugin
  would work
  - Some of the RPCs as of v1 of the CSI specification that would hence
  not be supported are,
    - Listing volumes and snapshots
    - Getting capacity of the backing Ceph cluster(s)

### Configuration specifics

#### `config.json` config map contents

```json
{
  "<clusterID>": {
    "monitors": [
      "IP/DNS:port",
      ...
    ]
  },
  ...
}
```

#### Annotated YAML for RBD StorageClass

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: fast
provisioner: csi-rbd
parameters:
  # [Optional] defaults to "ext4" allowed values depend on filesystems
  # supported
  fsType: ext4
  # [Optional] defaults to "2"
  imageFormat: "2"
  # [Optional], can be specified when imageFormat is used, defaults to "",
  #     possible values "layering"
  imageFeatures: "layering"
  # [Mandatory] carries the fsid of the Ceph cluster
  clusterID: <ID>
  # [Mandatory] carries the pool name to use in the Ceph cluster defined
  #     by clusterID
  pool: <pool-name>
  # [Mandatory] All of the following are mandatory, and as described in,
  #     https://kubernetes-csi.github.io/docs/secrets-and-credentials.html
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: default
  csi.storage.k8s.io/node-publish-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-publish-secret-namespace: default
```

#### Annotated YAML for CephFS StorageClass

**NOTE:** For terseness only deltas are detailed below

```yaml
  # [Mandatory]
  clusterID: <ID>
  # [Mandatory] carries the filesystem name to use in the Ceph cluster defined
  #     by clusterID
  fsname: <fs-name>

  provisionVolume: "true"
  # Ceph pool into which the volume shall be created
  # Required for provisionVolume: "true"
  pool: cephfs_data

  # Root path of an existing CephFS volume
  # Required for provisionVolume: "false"
  # rootPath: /absolute/path
```

### VolumeID and Ceph image/sub-directory naming

This section details how we will deal with image/FS names on Ceph cluster, when
provisioning volumes based on *volume_name* passed in by the CO, in the
CreateVolume request to the CS.

As per the CSI spec, creation requests come with a *volume_name* that is
controlled by the CO and is unique across different volume requests, and the
same for a given volume to maintain idempotent create requests.

The CSI plugin returns a *volume_id* that is passed to other plugin RPCs to
refer to the volume, and is generated by the plugin. Snippet from the spec
about the *volume_id*,

```
  // This field MUST contain enough information to uniquely identify
  // this specific volume vs all other volumes supported by this plugin.
  // This field SHALL be used by the CO in subsequent calls to refer to
  // this volume.
```

#### Name terminology

- **ImageName**: Name of the image as created on Ceph (rbd image name, or
  CephFS sub-directory name)
- **volume_id**: Identifier for the *ImageName* as shared with the CO system
- **csi-id**: Immutable CSI instance ID, to distinguish between different CSI
  instances, within or across CO systems, that may use the same Ceph cluster
- **volume_name**: Name of the volume as recognized by the CO system, passed in
  only during VolumeCreate operation
- **cluster-id**: Ceph cluster fsid
- **pool-name**: Ceph pool on which image is created
- **fsname**: Ceph FS filesystem name, on which volume is created

#### Name interpolation rules

1. *ImageName* is known only to Ceph, *volume_id* is known only to the CO system
1. We should be able to generate *ImageName* given a *volume_id*
  - This is needed in most NS operations, that receive only the *volume_id*
    and conditionally the *volume_context* and *publish_context*
  - *volume_context* and *publish_context* are only cached, and not
    essentially persisted, hence we cannot rely on payload from the same
    to complete the transformation
  - This transformation is needed by the CS specifically for DeleteVolume,
    DeleteSnapshot and ControllerExpandVolume operations
1. We should generate the same *ImageName*, when a Create request comes in with
  the same *volume_name*
  - CreateVolume may retry with the same *volume_name* on timeouts of the
    request, and hence we need to generate the same *ImageName* for a given
    *volume_name*
1. We should be able to distinguish *ImageName*s that come from an instance of
  the CSI plugin
  - When dealing with a common Ceph cluster shared across CO systems, we need
    the *ImageName* to be distinguishable, to avoid collisions and also to
    be able support/aid operations like ListVolumes/Snapshots
  - The same also applies when dealing with different CSI plugin instance in
    the same CO system, using the same Ceph cluster

#### Using Ceph OMaps for idempotent RPCs

To preserve the idempotent CreateVolume RPC requirement, across *ImageName* for
a given *volume_name*, it is proposed to use a per-CSI instance Ceph object named
`csi.volumes.[csi-id]`, with omap keys named `csi.volume.[volume_name]` with the
CO generated *volume_name*.

The value stored in `csi.volume.[volume_name]` key is the *ImageName* that is
used for the requested *volume_name*.

A create volume request would hence operate as follows,

- Use CO passed *volume_name* to generate the required key name
- Check if key exists and read the value of the same, if it does
- If key exists and value points to an image that satisfies the create request
  return with success
- If not, generate an *ImageName* and store the key and value into the omap
- Proceed with image creation operations as required

Racy key creations/accesses are prevented by locking on the *volume_name* that
the create request carries, thus ensuring that only one such transaction for
the key named with the *volume_name* is in flight at any given time.

One other advantage of using this omap is, list operations can be performed
more easily using the omap, rather than listing all images in the pool or the FS.

NOTE: For RBD plugin the RADOS objects and omaps would be stored in the same
pool as the image

NOTE: For CephFS plugin the RADOS objects would be stored in the metadata
pool of the fsname passed to the create request. With further restrictions
that the objects would be stored in a special `csicephfs` namespace on the
metadata pool.

#### Naming scheme

ImageName format: `csi-vol-[uuid]`

volume_id format: `[version]-[cluster-id]-[pool-id|fscid]-[uuid]`

The spec states strings are a maximum of 128 bytes in length. Both
*volume_name* and *volume_id* are strings, hence interpolating one into the
other can break the size rules and rules out such encoding schemes.

Due to the existence of the omap, we choose to create an ImageName that
is independent of any implicit *volume_name* or *csi-id* encoding within the name.

If an *ImageName* is generated that already exists in the pool/fsname, the create
request fails with required internal errors, and a retry would hence attempt to
generate a new *ImageName* with a new uuid as required.

The *volume_id* encoding helps identify the cluster, pool/fsname and *ImageName*
to use for RPCs that only carry the *volume_id* in its requests.

The `pool-id` and `fscid` are the respective pool and fsname IDs.

The `version` is the version of the `volume_id` encoding scheme, for future
encoding changes as the case may be.

The size of each element encoded into the *volume_id* are,
[4 bytes]-[MAX:37 Bytes]-[16 bytes]-[36 Bytes] respectively, thus bringing its
total length to 93 bytes maximum, within the given 128 byte limits.

## Appendix

### Solution Alternative-1

- Create a config map per Ceph cluster called `ceph-cluster-<cluster-fsid>` and
  a pair of secrets to go along with the same cluster called
  `ceph-cluster-<cluster-fsid>-provisioner-secret` and
  `ceph-cluster-<cluster-fsid>-publish-secret`
- Allow the CS and NS namespaces access to the above config maps and secrets,
  in addition to the namespace that is responsible for creating and
  maintaining it
- Details about the Ceph cluster are maintained in
  `ceph-cluster-<cluster-fsid>`. Details include cluster-id, MONs, pools, and
  other relevant data. Also, these and the corresponding secrets are updated
  when they change, to reflect current values
- The CSI plugin pod adds these config maps and secrets as volumes to its pod
  specification, thus ensuring auto-refresh of content when these are updated
  by the CO environment
- VolumeCreate requests come against a targeted Ceph cluster-id and pool,
  enabling the CSI plugin to locate the appropriate configuration information
  for the cluster and its secrets to act on the request
  - **NOTE:** The cluster-id and pool information is specified in the
    StorageClass for example
- The *volume_id* carries information regarding the *cluster_id* and pool,
  enabling *volume_id* based RPC requests to operate using the provided
  configuration information against the respective cluster
- Usual CSI spec based maps such as, *volume_context* and *publish_context*
  continue to carry information regarding required aspects about the
  provisioned volume to dependent RPCs
- The NS also has access to the config maps and secrets to satisfy required node
  level RPC requests

### Pros alternative-1

- Addresses removal of the current config map that maintains list of
  provisioned volumes
- Provides the ability for the CSI plugin to support all RPCs, as required
  information is present with the plugin across all Ceph clusters
- Safeguards secrets and cluster configuration information access to only
  required namespaces that need to manage and use it
- Provides the ability to have a single Ceph-CSI plugin instance per CO
  instance, even when the CO instance needs volumes provisioned from multiple
  Ceph clusters
- Also, does not take away the capability, where needed, to use multiple
  Ceph-CSI plugin instances (specialized using provisioner name) per CO
  instance
- Leaves the flexibility for the StorageClass to still override secrets, if such
  requirements arise
  - **NOTE:** The credentials stored for a cluster and passed to the CSI
    plugins CS and NS, should be able to list volumes/images created using
    other secrets (IOW, should act like a supervisor/root) to correctly
    support ListVolumes and ListSnapshots operations
- Provides the ability to deal with multi-cluster deployments, and possible
  best-fit provisioning in the **future**
  - Enables the ability for the CSI plugin to act on Topology based requests
  - Simplifies StorageClass definitions, leaving the choice of the
    cluster/pool to the CSI plugin

### Cons alternative-1

- Addition/Removal of Ceph cluster configuration, to the CSI plugin, requires
  that the pod specification be updated, and thus the CS and NS pods require
  to be restarted, in order to pick up the latest cluster details
- This scheme makes it so that 2 levels of secrets need to be maintained, one
  for the storage class and the other for the secret passed to the plugin.
  Further, the plugin secret capabilities and the storage class passed secrets
  capabilities are the same.

### Solution Alternative-2

#### Summary alternative-2

- Create a config map (**ceph-clusters**) containing all Ceph clusters that the
  CSI plugin may need to provision storage from, and keep this updated with
  new and changing information as required (by a human or an automated
  operator).
- Further, create a secret blob (**ceph-cluster-secrets**), containing secrets
  for each Ceph cluster in the above **ceph-clusters**. This is updated as the
  secrets change or new clusters are added.
- *ceph-clusters* is used by the CS, and required information passed to the NS
  via *volume_context* and *publish_context* maps, from CreateVolume and
  ControllerPublishVolume responses respectively (as per the CSI spec)
  - *publish_context* carries the latest MON information, as this is invoked
    each time the volume needs to be published
  - *volume_context* can carry other required information (e.g: cluster ID,
    pool name), information that remains immutable for the lifetime of the
    provisioned volume
- *ceph-clusters-secret* is used by the CS and the NS, when the required secrets
  are not present in the CSI secrets fields (e.g: when not specified in the
  StorageClass)

#### Pros alternative-2

- Addresses removal of the current config map that maintains list of provisioned
  volumes
- Adding new Ceph cluster details to *ceph-clusters* does not need the CS or NS
  pods to be restarted, as the config map updates will eventually refresh
  within the container
- StorageClass can still specify secrets in case it wants it to be specialized,
  - Although, we will not get the secret information in the following
    operations, ListVolumes, ListSnapshots, and GetCapacity
- CS and NS pods do not need a restart when clusters are added or removed from
  the global config map

#### Cons alternative-2

- *ceph-clusters* config map either needs to be exposed to namespaces that need
  to manage their cluster information instance (say multiple Rook instances,
  if each is managing a Ceph cluster of its own)
- *ceph-cluster-secrets* also, suffers the above constraint and can be a
  security concern as different namespace admins can read/write others secrets

#### Other notes alternative-2

- GetCapacity request does not have targeted cluster information, hence
  response would be across all clusters in *ceph-clusters* that satisfy the
  passed in Topology
- ListVolumes and ListSnapshots operations, will return all Volumes across all
  clusters that are a part of *ceph-clusters*
  - Assuming ListSnapshots comes with no *source_volume_id* or *snapshot_id*
    as these are optional fields

#### Alternative-2 configuration details

##### JSON representation of *ceph-clusters*

```json
{
  "<clusterID>": {
    "monitors": [
      "IP/DNS:port",
      ...
    ],
  },
  ...
}
```

##### JSON representation of *ceph-cluster-secrets*

```json
{
  "<clusterID>": {
    "adminId": "kube",
    "adminSecretName": "ceph-secret",
    "adminSecretNamespace": "kube-system",

    "userId": "kube",
    "userSecretName": "ceph-secret-user",
    "userSecretNamespace": "default",
  },
  ...
}
```

### Solution Alternative-3

#### Summary alternative-3

- Primary aim of this alternative is to avoid sharing the *ceph-cluster-secrets*
  across namespaces as in Alternative-1
- Hence, move *ceph-cluster-secrets* to being mandatory in the StorageClass,
  avoiding sharing cross-cluster secrets in a single secrets blob

#### Pros alternative-3

- Retains all of the prior pros in Alternative-1
- Does not leak secrets across separate Ceph clusters, possibly managed by
  different namespaces as there exists no *ceph-cluster-secrets*

#### Cons alternative-3

- *ceph-cluster-secrets* is absent and hence following operations **cannot**
  be supported
  - ListVolumes, ListSnapshots, and GetCapacity
  - **NOTE:** Unless listing rbd and CephFS volumes can be done without
    credentials
- Bloats every StorageClass making secrets mandatory
- Retains other cons as in Alternative-1

### Solution Alternative-4

#### Summary alternative-4

- Address *ceph-clusters* being accessible to all namespaces that need to
  manage it
- Move the information in *ceph-clusters* to secrets in the StorageClass (as in
  monValueFromSecret configuration implementation in the code as of this
  writing)

#### Pros and cons alternative-4

- Cons remains the same, with further added complexity in the StorageClass and
  its secrets
- Removes the con in Alternate-1 and 2 where the *ceph-clusters* information is
  accessible across namespaces

### Solution Alternative-5

#### Summary alternative-5

- Create a CSI plugin instance per Ceph cluster, with its own unique provisioner
  name
- Each CSI plugin instance can now have its own *ceph-cluster* and
  *ceph-secrets* config maps, where *ceph-cluster* contains information
  regarding exactly one cluster and similarly the *ceph-secret* for the same
- StorageClass points to the provisioner name and does not need secrets,
  cluster-id and may optionally choose to contain pool-name (can be omitted,
  if we choose to to allocate from an available pool in the cluster rather
  than a specific pool, based on the CreateVolume request parameters and
  Topology)

#### Pros alternative-5

- Can support all Operations, as cluster information and secrets are per
  instance
- The secrets and cluster information need not be shared across multiple
  namespaces
- StorageClass becomes leaner, but still needs at least as many StorageClasses
  as there are clusters backing the CO environment
- Allows for future merging of the CSI instances, reverting to the StorageClass
  as in the proposed solution

#### Cons alternative-5

- We will have as many NodeService entities as there are clusters in each node
  of the CO environment. This may consume more resources
- We cannot, deal with multiple Ceph clusters backed by a single CSI plugin
  instance, due to the strict 1:1 mapping between the same, and thus do any
  form of intelligent provisioning across Ceph clusters for a request
- **Needs Investigation** Does this hinder Topology based volume provisioning?
  As each instance is stuck to single Ceph cluster, we cannot provision from
  different Topologies (each with their own Ceph clusters)
