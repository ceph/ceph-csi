# CSI RBD Plugin

> **Note:** The preferred deployment method for Kubernetes is the
> [Ceph-CSI Operator](https://ceph.github.io/ceph-csi-operator), which manages
> driver lifecycle, upgrades, and configuration automatically. The manual steps
> below are for advanced or non-operator deployments.

The RBD CSI plugin is able to provision new RBD images and
attach and mount those to workloads.

## Building

CSI plugin can be compiled in a form of a binary file or in a form of a
Docker image. When compiled as a binary file, the result is stored in
`_output/` directory with the name `cephcsi`. When compiled as an image, it's
stored in the local Docker image store with name `cephcsi`.

Building binary:

```bash
make cephcsi
```

Building Docker image:

```bash
make image-cephcsi
```

## Configuration

**Available command line arguments:**

| Option                   | Default value                 | Description                                                                                                                                                                                                                                                                          |
| ------------------------ | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--endpoint`             | `unix:///tmp/csi.sock`        | CSI endpoint, must be a UNIX socket                                                                                                                                                                                                                                                  |
| `--csi-addons-endpoint`  | `unix:///tmp/csi-addons.sock` | CSI-Addons endpoint, must be a UNIX socket                                                                                                                                                                                                                                           |
| `--drivername`           | `rbd.csi.ceph.com`            | Name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)                                                                                                                                                                                   |
| `--nodeid`               | _empty_                       | This node's ID                                                                                                                                                                                                                                                                       |
| `--type`                 | _empty_                       | Driver type: `[rbd/cephfs]`. If the driver type is set to  `rbd` it will act as a `rbd plugin` or if it's set to `cephfs` will act as a `cephfs plugin`                                                                                                                              |
| `--instanceid`           | "default"                     | Unique ID distinguishing this instance of Ceph CSI among other instances, when sharing Ceph clusters across CSI instances for provisioning                                                                                                                                           |
| `--pidlimit`             | _0_                           | Configure the PID limit in cgroups. The container runtime can restrict the number of processes/tasks which can cause problems while provisioning (or deleting) a large number of volumes. A value of `-1` configures the limit to the maximum, `0` does not configure limits at all. |
| `--metricsport`          | `8080`                        | TCP port for liveness metrics requests                                                                                                                                                                                                                                               |
| `--metricspath`          | `"/metrics"`                  | Path of prometheus endpoint where metrics will be available                                                                                                                                                                                                                          |
| `--polltime`             | `"60s"`                       | Time interval in between each poll                                                                                                                                                                                                                                                   |
| `--timeout`              | `"3s"`                        | Probe timeout in seconds                                                                                                                                                                                                                                                             |
| `--clustername`          | _empty_                       | Cluster name to set on RBD image                                                                                                                                                                                                                                                     |
| `--domainlabels`         | _empty_                       | Kubernetes node labels to use as CSI domain labels for topology aware provisioning, should be a comma separated value (ex:= "failure-domain/region,failure-domain/zone")                                                                                                             |
| `--rbdhardmaxclonedepth` | `8`                           | Hard limit for maximum number of nested volume clones that are taken before a flatten occurs                                                                                                                                                                                         |
| `--rbdsoftmaxclonedepth` | `4`                           | Soft limit for maximum number of nested volume clones that are taken before a flatten occurs                                                                                                                                                                                         |
| `--skipforceflatten`     | `false`                       | skip image flattening on kernel < 5.2 which support mapping of rbd images which has the deep-flatten feature                                                                                                                                                                         |
| `--maxsnapshotsonimage`  | `450`                         | Maximum number of snapshots allowed on rbd image without flattening                                                                                                                                                                                                                  |
| `--setmetadata`          | `true`                       | **Deprecated.** Set metadata on volume. This flag will be removed in a future release.                                                                                                                                                                                               |
| `--enable-read-affinity` | `false`                       | enable read affinity                                                                                                                                                                                                                                                                 |
| `--crush-location-labels`| _empty_                       | Kubernetes node labels that determine the CRUSH location the node belongs to, separated by ','.<br>`Note: These labels will be replaced if crush location labels are defined in the ceph-csi-config ConfigMap for the specific cluster.`                                                                                                                                                                                       |
| `--logslowopinterval`    | `30s`                         | Log slow operations at the specified rate. Operation is considered slow if it outlives its deadline.                                                                                                                                                                                                                                                                                                                           |
| `--feature-gates`        | _empty_                       | Comma-separated list of feature gates (e.g., `SlowGRPCRestart=false`). Available gates: `SlowGRPCRestart` (default: `true`) — restart the process when a unary gRPC call is stuck for more than 10 minutes.                                                                                                                                                                                                                   |

**Available volume parameters:**

| Parameter                                                                                           | Required             | Description                                                                                                                                                                                                                                                                                        |
|-----------------------------------------------------------------------------------------------------|----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `clusterID`                                                                                         | yes                  | String representing a Ceph cluster, must be unique across all Ceph clusters in use for provisioning, cannot be greater than 36 bytes in length, and should remain immutable for the lifetime of the Ceph cluster in use                                                                            |
| `pool`                                                                                              | yes                  | Ceph pool into which the RBD image shall be created                                                                                                                                                                                                                                                |
| `dataPool`                                                                                          | no                   | Ceph pool used for the data of the RBD images.                                                                                                                                                                                                                                                     |
| `volumeNamePrefix`                                                                                  | no                   | Prefix to use for naming RBD images (defaults to `csi-vol-`).                                                                                                                                                                                                                                      |
| `snapshotNamePrefix`                                                                                | no                   | Prefix to use for naming RBD snapshot images (defaults to `csi-snap-`).                                                                                                                                                                                                                            |
| `imageFeatures`                                                                                     | no                   | RBD image features. CSI RBD currently supports `layering`, `journaling`, `exclusive-lock`, `object-map`, `fast-diff`, `deep-flatten` features. deep-flatten is added for cloned images. Refer <https://docs.ceph.com/en/latest/rbd/rbd-config-ref/#image-features> for image feature dependencies. |
| `mkfsOptions`                                                                                       | no                   | Options to pass to the `mkfs` command while creating the filesystem on the RBD device. Check the man-page for the `mkfs` command for the filesystem for more details. When `mkfsOptions` is set here, the defaults will not be used, consider including them in this parameter.                    |
| `tryOtherMounters`                                                                                  | no                   | Specifies whether to try other mounters in case if the current mounter fails to mount the rbd image for any reason                                                                                                                                                                                 |
| `mapOptions`                                                                                        | no                   | Map options to use when mapping rbd image. See [krbd](https://docs.ceph.com/docs/master/man/8/rbd/#kernel-rbd-krbd-options) and [nbd](https://docs.ceph.com/docs/master/man/8/rbd-nbd/#options) options.                                                                                           |
| `unmapOptions`                                                                                      | no                   | Unmap options to use when unmapping rbd image. See [krbd](https://docs.ceph.com/docs/master/man/8/rbd/#kernel-rbd-krbd-options) and [nbd](https://docs.ceph.com/docs/master/man/8/rbd-nbd/#options) options.                                                                                       |
| `csi.storage.k8s.io/provisioner-secret-name`, `csi.storage.k8s.io/node-stage-secret-name`           | yes (for Kubernetes) | name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value                                                                                                                                                                                |
| `csi.storage.k8s.io/provisioner-secret-namespace`, `csi.storage.k8s.io/node-stage-secret-namespace` | yes (for Kubernetes) | namespaces of the above Secret objects                                                                                                                                                                                                                                                             |
| `mounter`                                                                                           | no                   | if set to `rbd-nbd`, use `rbd-nbd` on nodes that have `rbd-nbd` and `nbd` kernel modules to map rbd images                                                                                                                                                                                         |
| `encrypted`                                                                                         | no                   | disabled by default, use `"true"` to enable either LUKS or fscrypt encryption on PVC and `"false"` to disable it. **Do not change for existing storageclasses**                                                                                                                                                      |
| `encryptionSectorSize`                                                                              | no                   | set the sector size that is used to perform I/O. Size must be a power of two and in between 512 and 4096. Typical values are 4096 and 512.                                                                   |
| `encryptionCipher`                                                                                  | no                   | set the cipher that is used for the volume encryption. **Supported Keywords**: `aes-xts-plain64` (default), `serpent-xts-plain64`,  `aes-xts-random`, `serpent-xts-random`                                                                           |
| `encryptionKeySize`                                                                                 | no                   | set the key size used for encryption. Typical key sizes are 128, 256 and 512. Default 256 (or 512 when using an `xts` cipher mode)                                                                            |
| `integrityMode`                                                                                     | no                   | set and enable the integrity verification for volume encryption.  **Supported Keywords**: `hmac-sha256`, `hmac-sha512`                                                                            |
| `encryptionKMSID`                                                                                   | no                   | required if encryption is enabled and a kms is used to store passphrases                                                                                                                                                                                                                           |
| `encryptionType`                                                                                    | no                   | Either `block` or `file`. If unset or `block` use LUKS block device encryption. If `file` use ext4 fscrypt to encrypt on the file system level (requires kernel support).                                                                                                                           |
| `stripeUnit`                                                                                        | no                   | stripe unit in bytes                                                                                                                                                                                                                                                                               |
| `stripeCount`                                                                                       | no                   | objects to stripe over before looping                                                                                                                                                                                                                                                              |
| `objectSize`                                                                                        | no                   | object size in bytes                                                                                                                                                                                                                                                                               |
| `baseIops`                                                                                          | no                   | the base limit of operations per second |
| `maxIops`                                                                                           | no                   | the max limit of operations per second  |
| `baseReadIops`                                                                                      | no                   | the base limit of read operations per second |
| `maxReadIops`                                                                                       | no                   | the max limit of read operations per second |
| `baseWriteIops`                                                                                     | no                   | the base limit of write operations per second |
| `maxWriteIops`                                                                                      | no                   | the max limit of write operations per second |
| `baseBps`                                                                                           | no                   | the base limit of bytes per second |
| `maxBps`                                                                                            | no                   | the max limit of bytes per second |
| `baseReadBps`                                                                                       | no                   | the base limit of read bytes per second |
| `maxReadBps`                                                                                        | no                   | the max limit of read bytes per second |
| `baseWriteBps`                                                                                      | no                   | the base limit of write bytes per second |
| `maxWriteBps`                                                                                       | no                   | the max limit of write bytes per second |
| `iopsPerGiB`                                                                                        | no                   | the limit of operations per GiB |
| `readIopsPerGiB`                                                                                    | no                   | the limit of read operations per GiB |
| `writeIopsPerGiB`                                                                                   | no                   | the limit of write operations per GiB |
| `bpsPerGiB`                                                                                         | no                   | the limit of bytes per GiB |
| `readBpsPerGiB`                                                                                     | no                   | the limit of read bytes per GiB |
| `writeBpsPerGiB`                                                                                    | no                   | the limit of write bytes per GiB |
| `baseVolSizeBytes`                                                                                  | no                   | the min size of volume what use to calculate qos beased on capacity |
| `extraDeploy` | no | array of extra objects to deploy with the release |

**NOTE:** An accompanying CSI configuration file, needs to be provided to the
running pods. Refer to [Creating CSI configuration](https://github.com/ceph/ceph-csi/blob/devel/examples/README.md#creating-csi-configuration)
for more information.

**NOTE:** A suggested way to populate and retain uniqueness of the clusterID is
to use the output of `ceph fsid` of the Ceph cluster to be used for
provisioning.

**Required secrets:**

User credentials, with required access to the pool being used in the storage class,
is required for provisioning new RBD images.

## Deployment with Kubernetes

Requires Kubernetes 1.14+

Use the [rbd templates](../../deploy/rbd/kubernetes)

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged`
flag must be set to true for both the API server and the kubelet). Moreover, as
stated in the [mount propagation
docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation),
the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in `deploy/rbd/kubernetes`.

**Create CSIDriver object:**

```bash
kubectl create -f csidriver.yaml
```

**Deploy RBACs for sidecar containers and node plugins:**

```bash
kubectl create -f csi-provisioner-rbac.yaml
kubectl create -f csi-nodeplugin-rbac.yaml
```

Those manifests deploy service accounts, cluster roles and cluster role
bindings. These are shared for both RBD and CephFS CSI plugins, as they require
the same permissions.

**Deploy ConfigMap for CSI plugins:**

```bash
kubectl create -f csi-config-map.yaml
```

The configmap deploys an empty CSI configuration that is mounted as a volume
within the Ceph CSI plugin pods. To add a specific Ceph clusters configuration
details, refer to [Creating CSI configuration for RBD based
provisioning](https://github.com/ceph/ceph-csi/blob/devel/examples/README.md#creating-csi-configuration)
for more information.

**Deploy Ceph configuration ConfigMap for CSI pods:**

```bash
kubectl create -f ../../ceph-conf.yaml
```

**Deploy prerequisites for CSI Snapshot:**

If you intend to use the snapshot functionality in Kubernetes cluster,
please refer to [snap-clone.md](../snap-clone.md#prerequisite)

**Deploy CSI sidecar containers:**

```bash
kubectl create -f csi-rbdplugin-provisioner.yaml
```

Deploys deployment of provision which includes external-provisioner
,external-attacher,csi-snapshotter sidecar containers and CSI RBD plugin.

**Deploy RBD CSI driver:**

```bash
kubectl create -f csi-rbdplugin.yaml
```

Deploys a daemon set with two containers: CSI node-driver-registrar and the CSI
RBD driver.

**NOTE:**
In case you want to use a different release version, replace canary with the
release version in the
[provisioner](../../deploy/rbd/kubernetes/csi-rbdplugin-provisioner.yaml)
and [nodeplugin](../../deploy/rbd/kubernetes/csi-rbdplugin.yaml) YAMLs.

```yaml
# for stable functionality replace canary with latest release version
    image: quay.io/cephcsi/cephcsi:canary
```

Check the release version [here.](../../README.md#ceph-csi-container-images-and-release-compatibility)

## Verifying the deployment in Kubernetes

After successfully completing the steps above, you should see output similar to this:

```bash
$ kubectl get all
NAME                              READY     STATUS    RESTARTS   AGE
pod/csi-rbdplugin-fptqr           3/3       Running   0          21s
pod/csi-rbdplugin-provisioner-0   5/5       Running   0          22s

NAME                                TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)     AGE
service/csi-rbdplugin-provisioner   ClusterIP   10.104.2.130   <none>        8080/TCP   23s
...
```

Once the CSI plugin configuration is updated with details from a Ceph cluster of
choice, you can try deploying a demo pod from examples/rbd using the
instructions
[provided](https://github.com/ceph/ceph-csi/blob/devel/examples/README.md#deploying-the-storage-class)
to test the deployment further.

## Deployment with Helm

The same requirements from the Kubernetes section apply here, i.e. Kubernetes
version, privileged flag and shared mounts.

The Helm chart is located in `charts/ceph-csi-rbd`.

**Deploy Helm Chart:**

[See the Helm chart readme for installation instructions.](../../charts/ceph-csi-rbd/README.md)

## Read Affinity using crush locations for RBD volumes

Ceph CSI supports mapping RBD volumes with krbd options
`"read_from_replica=localize,crush_location=type1:value1|type2:value2"` to
allow serving reads from the most local OSD (according to OSD locations as
defined in the CRUSH map).
Refer [krbd-options](https://docs.ceph.com/en/latest/man/8/rbd/#kernel-rbd-krbd-options)
for more details.

This can be enabled by adding labels to Kubernetes nodes like
`"topology.io/region=east"` and `"topology.io/zone=east-zone1"` and
passing command line arguments `"--enable-read-affinity=true"` and
`"--crush-location-labels=topology.io/zone,topology.io/region"` to Ceph CSI
RBD daemonset pod "csi-rbdplugin" container, resulting in Ceph CSI adding
`"--options read_from_replica=localize,crush_location=zone:east-zone1|region:east"`
krbd options during rbd map operation.
If enabled, this option will be added to all RBD volumes mapped by Ceph CSI.
Well known labels can be found
[here](https://kubernetes.io/docs/reference/labels-annotations-taints/).

Read affinity can be configured for individual clusters within the
`ceph-csi-config` ConfigMap. This allows configuring the crush location labels
for each ceph cluster separately. The crush location labels specified in the
ConfigMap will supersede  those provided via command line argument
`--crush-location-labels`.

>Note: Label values will have all its dots `"."` normalized with dashes `"-"`
in order for it to work with ceph CRUSH map.

## Encryption for RBD volumes

Volumes provisioned with Ceph RBD do not have encryption by default. It is
possible to encrypt them with ceph-csi by using LUKS encryption, including
Key Management System (KMS) support.

See [Encrypted Volumes](../encrypted-volumes.md) for the encryption
life-cycle, configuration and KMS setup details.

## Enable librbd logs for RBD operations

For debugging, a user might need the librbd logs for each RBD command executed
by ceph-csi through go-ceph. Therefore, one can enable these logs in the
`csi-rbdplugin` container in `csi-rbdplugin-provisioner-*`, `csi-rbdplugin-*`
controller, nodeplugin pod respectively by following the steps mentioned below:

In the [ceph-conf](../../deploy/ceph-conf.yaml) configmap, uncomment the
`debug_rbd` and `log_to_stderr` key/value pairs and update the configmap in the
cluster in the same namespace as the `csi-*` pods.

```bash
kubectl apply -f ceph-conf.yaml
```

And, if the ``ceph-config` configmap already exists from previous installation
then, edit the configmap to add `log_to_stderr = true`,`debug_rbd = 30`
values to `ceph.conf` field using:

```bash
kubectl edit cm ceph-config
```

This will update the `ceph.conf` of the underlying ceph cluster to enable
librbd logs in `csi-rbdplugin` container.

## Changed Block Tracking (CBT)

> **Warning**: Requires Ceph version that supports RBD snap diff by ID feature.
For details, see ceph tracker:
["diff-iterate by snap ID"](https://tracker.ceph.com/issues/65720).

Ceph-CSI implements Changed Block Tracking (CBT) for RBD volumes using
the SnapshotMetadataService (SMS) APIs. This feature enables efficient
and reliable differential backup of data stored in CSI volumes.

The feature is exposed through the CSI Controller Server's
SnapshotMetadataService APIs with two primary operations:

* `GetMetadataAllocated`:

   * Streams metadata about allocated blocks in a snapshot
   * Useful for full backup operations
   * Returns block ranges that contain actual data

* `GetMetadataDelta`:

   * Streams metadata about block differences between two snapshots
   * Optimal for incremental backups
   * Returns only blocks that changed between snapshots

Additional Resources:

* CSI Differential Snapshot for Block Volumes KEP:
  [kep-3314](https://github.com/kubernetes/enhancements/issues/3314)

* External Snapshot Metadata sidecar project:
  [repository](https://github.com/kubernetes-csi/external-snapshot-metadata)

## Kubernetes ServiceAccount Based Volume Access

Ceph-CSI supports optionally restricting RBD volume
access to specific Kubernetes ServiceAccounts. When
configured, only Pods running with one of the allowed
ServiceAccounts can mount the volume. One or more
ServiceAccounts can be specified as a comma-separated
list. This feature uses RBD image metadata to store
the restriction and the CSI
[`podInfoOnMount`][pod-info-on-mount] mechanism to identify
the Pod's ServiceAccount during mount.

[pod-info-on-mount]:
<https://kubernetes-csi.github.io/docs/pod-info.html#pod-info-on-mount-with-csi-driver-object>

### How it works

1. A storage admin sets the
   `.rbd.csi.ceph.com/serviceaccount` metadata on an
   RBD image to specify the allowed ServiceAccount
   name(s) as a comma-separated list.
1. During `ControllerPublishVolume`, Ceph-CSI reads this metadata and passes
   it to the node via publish context.
1. During `NodePublishVolume`, Ceph-CSI splits the
   comma-separated value and checks whether the Pod's
   ServiceAccount (provided via volume context by
   Kubelet) matches any entry.
1. If the ServiceAccount was set in metadata and does
   not match any of the allowed ServiceAccounts, the
   mount is rejected with a `PermissionDenied` error.

### Prerequisites

The [`podInfoOnMount`][pod-info-on-mount] field must be
set to `true` in the CSIDriver spec so that Kubelet
passes Pod information (including ServiceAccount name)
in the volume context during `NodePublishVolume`.
Without this, the restriction cannot be enforced and
all mounts are allowed.

This feature requires controller-publish-secret set in storageclass
for newer PVCs. For existing PVCs, the workaround mentioned
[here](../design/proposals/non-graceful-node-shutdown.md#workaround-for-older-pvs)
can be used.

### Setting the restriction on an RBD image

Use the `rbd image-meta set` command to set the
allowed ServiceAccount(s). Multiple ServiceAccounts
can be specified as a comma-separated list:

```bash
rbd image-meta set <pool>/<image> \
  .rbd.csi.ceph.com/serviceaccount \
  <service-account-name>[,<service-account-name>...]
```

For example, to restrict a volume to the
`my-app-sa` ServiceAccount:

```bash
rbd image-meta set mypool/csi-vol-abc123 \
  .rbd.csi.ceph.com/serviceaccount my-app-sa
```

To allow multiple ServiceAccounts:

```bash
rbd image-meta set mypool/csi-vol-abc123 \
  .rbd.csi.ceph.com/serviceaccount \
  my-app-sa,my-worker-sa
```

### Removing the restriction

To remove the restriction and allow any ServiceAccount to mount the volume:

```bash
rbd image-meta remove <pool>/<image> .rbd.csi.ceph.com/serviceaccount
```

All the Pods using the PVC should be scaled down completely and
then scaled up for removing the restriction after
removing metadata from the image.
