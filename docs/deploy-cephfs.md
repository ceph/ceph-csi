
# CSI CephFS plugin

The CSI CephFS plugin is able to both provision new CephFS volumes
and attach and mount existing ones to workloads.

## Building

CSI plugin can be compiled in the form of a binary file or in the form
of a Docker image.
When compiled as a binary file, the result is stored in `_output/`
directory with the name `cephcsi`.
When compiled as an image, it's stored in the local Docker image store
with name `cephcsi`.

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

| Option                    | Default value               | Description                                                                                                                                                                                                                                                                          |
| ------------------------- | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--endpoint`              | `unix://tmp/csi.sock`       | CSI endpoint, must be a UNIX socket                                                                                                                                                                                                                                                  |
| `--drivername`            | `cephfs.csi.ceph.com`       | Name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)                                                                                                                                                                                   |
| `--nodeid`                | _empty_                     | This node's ID                                                                                                                                                                                                                                                                       |
| `--type`                  | _empty_                     | Driver type: `[rbd/cephfs]`. If the driver type is set to  `rbd` it will act as a `rbd plugin` or if it's set to `cephfs` will act as a `cephfs plugin`                                                                                                                                        |
| `--instanceid`            | "default"                   | Unique ID distinguishing this instance of Ceph CSI among other instances, when sharing Ceph clusters across CSI instances for provisioning                                                                                                                                           |
| `--pluginpath`            | "/var/lib/kubelet/plugins/" | The location of cephcsi plugin on host                                                                                                                                                                                                                                               |
| `--pidlimit`              | _0_                         | Configure the PID limit in cgroups. The container runtime can restrict the number of processes/tasks which can cause problems while provisioning (or deleting) a large number of volumes. A value of `-1` configures the limit to the maximum, `0` does not configure limits at all. |
| `--metricsport`           | `8080`                      | TCP port for liveness metrics requests                                                                                                                                                                                                                                               |
| `--metricspath`           | `/metrics`                  | Path of prometheus endpoint where metrics will be available                                                                                                                                                                                                                          |
| `--polltime`              | `60s`                       | Time interval in between each poll                                                                                                                                                                                                                                                   |
| `--timeout`               | `3s`                        | Probe timeout in seconds                                                                                                                                                                                                                                                             |
| `--clustername`           | _empty_                     | Cluster name to set on subvolume                                                                                                                                                                                                                                                     |
| `--forcecephkernelclient` | `false`                     | Force enabling Ceph Kernel clients for mounting on kernels < 4.17                                                                                                                                                                                                                    |
| `--kernelmountoptions`    | _empty_                     | Comma separated string of mount options accepted by cephfs kernel mounter.<br>`Note: These options will be replaced if kernelMountOptions are defined in the ceph-csi-config ConfigMap for the specific cluster.`                                                                                                                                                                                                               |
| `--fusemountoptions`      | _empty_                     | Comma separated string of mount options accepted by ceph-fuse mounter.<br>`Note: These options will be replaced if fuseMountOptions are defined in the ceph-csi-config ConfigMap for the specific cluster.`                                                                                                                                                                                                               |
| `--domainlabels`          | _empty_                     | Kubernetes node labels to use as CSI domain labels for topology aware provisioning, should be a comma separated value (ex:= "failure-domain/region,failure-domain/zone")                                                                                                             |
| `--enable-read-affinity` | `false`                       | enable read affinity                                                                                                                                                                                                                                                                 |
| `--crush-location-labels`| _empty_                       | Kubernetes node labels that determine the CRUSH location the node belongs to, separated by ','.<br>`Note: These labels will be replaced if crush location labels are defined in the ceph-csi-config ConfigMap for the specific cluster.`                                                                                                                                                                                       |
| `--radosnamespacecephfs`| _empty_                       | CephFS RadosNamespace used to store CSI specific objects and keys.                                                                                                                               |

**NOTE:** The parameter `-forcecephkernelclient` enables the Kernel
CephFS mounter on kernels < 4.17.
**This is not recommended/supported if the kernel does not support quota.**

**Available environmental variables:**

`KUBERNETES_CONFIG_PATH`: if you use `k8s_configmap` as metadata store, specify
the path of your k8s config file (if not specified, the plugin will assume
you're running it inside a k8s cluster and find the config itself).

**Available volume parameters:**

| Parameter                                                                                           | Required       | Description                                                                                                                                                                                                             |
|-----------------------------------------------------------------------------------------------------|----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `clusterID`                                                                                         | yes            | String representing a Ceph cluster, must be unique across all Ceph clusters in use for provisioning, cannot be greater than 36 bytes in length, and should remain immutable for the lifetime of the Ceph cluster in use |
| `fsName`                                                                                            | yes            | CephFS filesystem name into which the volume shall be created                                                                                                                                                           |
| `mounter`                                                                                           | no             | Mount method to be used for this volume. Available options are `kernel` for Ceph kernel client and `fuse` for Ceph FUSE driver. Defaults to "default mounter".                                                          |
| `pool`                                                                                              | no             | Ceph pool into which volume data shall be stored                                                                                                                                                                        |
| `volumeNamePrefix`                                                                                  | no             | Prefix to use for naming subvolumes (defaults to `csi-vol-`).                                                                                                                                                           |
| `snapshotNamePrefix`                                                                                | no             | Prefix to use for naming snapshots (defaults to `csi-snap-`)                                                                                                                                                            |
| `backingSnapshot`                                                                                   | no             | Boolean value. The PVC shall be backed by the CephFS snapshot specified in its data source. `pool` parameter must not be specified. (defaults to `true`)                                                               |
| `kernelMountOptions`                                                                                | no             | Comma separated string of mount options accepted by cephfs kernel mounter, by default no options are passed. Check man mount.ceph for options.                                                                          |
| `fuseMountOptions`                                                                                  | no             | Comma separated string of mount options accepted by ceph-fuse mounter, by default no options are passed.                                                                                                                |
| `csi.storage.k8s.io/provisioner-secret-name`, `csi.storage.k8s.io/node-stage-secret-name`           | for Kubernetes | Name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value                                                                                                     |
| `csi.storage.k8s.io/provisioner-secret-namespace`, `csi.storage.k8s.io/node-stage-secret-namespace` | for Kubernetes | Namespaces of the above Secret objects                                                                                                                                                                                  |
| `encrypted`                                                                                         | no             | disabled by default, use `"true"` to enable fscrypt encryption on PVC and `"false"` to disable it. **Do not change for existing storageclasses**                                                                          |
| `encryptionKMSID`                                                                                   | no             | required if encryption is enabled and a kms is used to store passphrases                                                                                                                                                |
| `extraDeploy` | no | array of extra objects to deploy with the release |

**NOTE:** An accompanying CSI configuration file, needs to be provided to the
running pods. Refer to [Creating CSI configuration](../examples/README.md#creating-csi-configuration)
for more information.

**NOTE:** A suggested way to populate and retain uniqueness of the clusterID is
to use the output of `ceph fsid` of the Ceph cluster to be used for
provisioning.

**Required secrets for provisioning:**
Admin credentials are required for provisioning new volumes

* `adminID`: ID of an admin client
* `adminKey`: key of the admin client

**Required secrets for statically provisioned volumes:**
User credentials with access to an existing volume

* `userID`: ID of a user client
* `userKey`: key of a user client

Notes on volume size: when provisioning a new volume, `max_bytes` quota
attribute for this volume will be set to the requested volume size (see [Ceph
quota documentation](http://docs.ceph.com/docs/nautilus/cephfs/quota/)). A request
for a zero-sized volume means no quota attribute will be set.

## Deployment with Kubernetes

Requires Kubernetes 1.14+

Use the [cephfs templates](../deploy/cephfs/kubernetes)

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged`
flag must be set to true for both the API server and the kubelet). Moreover, as
stated in the [mount propagation
docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation),
the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in `deploy/cephfs/kubernetes`.

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
details, refer to [Creating CSI configuration](../examples/README.md#creating-csi-configuration)
for more information.

**Deploy Ceph configuration ConfigMap for CSI pods:**

```bash
kubectl create -f ../../ceph-conf.yaml
```

**Deploy prerequisites for CSI Snapshot:**

If you intend to use the snapshot functionality in Kubernetes cluster,
please refer to [snap-clone.md](./snap-clone.md#prerequisite)

**Deploy CSI sidecar containers:**

```bash
kubectl create -f csi-cephfsplugin-provisioner.yaml
```

Deploys deployment of provision which includes external-provisioner
,external-attacher for CSI CephFS.

**Deploy CSI CephFS driver:**

```bash
kubectl create -f csi-cephfsplugin.yaml
```

Deploys a daemon set with two containers: CSI node-driver-registrar and
the CSI CephFS driver.

**NOTE:**
In case you want to use a different release version, replace canary with the
release version in the
[provisioner](../deploy/cephfs/kubernetes/csi-cephfsplugin-provisioner.yaml)
and [nodeplugin](../deploy/cephfs/kubernetes/csi-cephfsplugin.yaml) YAMLs.

```yaml
# for stable functionality replace canary with latest release version
    image: quay.io/cephcsi/cephcsi:canary
```

Check the release version [here.](../README.md#ceph-csi-container-images-and-release-compatibility)

## Verifying the deployment in Kubernetes

After successfully completing the steps above, you should see output similar to this:

```bash
$ kubectl get all
NAME                                 READY     STATUS    RESTARTS   AGE
pod/csi-cephfsplugin-provisioner-0   4/4       Running   0          25s
pod/csi-cephfsplugin-rljcv           3/3       Running   0          24s

NAME                                   TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)     AGE
service/csi-cephfsplugin-provisioner   ClusterIP   10.101.78.75     <none>        8080/TCP   26s
...
```

Once the CSI plugin configuration is updated with details from a Ceph cluster of
choice, you can try deploying a demo pod from examples/cephfs using the
instructions [provided](../examples/README.md#deploying-the-storage-class) to
test the deployment further.

### Notes on volume deletion

Dynamically provisioned volumes are deleted by the driver, when requested to
do so. Statically provisioned volumes, from plugin versions less than or
equal to 1.0.0, are a no-op when a delete operation is performed against the
same, and are expected to be deleted on the Ceph cluster by the user.

## Deployment with Helm

The same requirements from the Kubernetes section apply here, i.e. Kubernetes
version, privileged flag and shared mounts.

The Helm chart is located in `charts/ceph-csi-cephfs`.

**Deploy Helm Chart:**

[See the Helm chart readme for installation instructions.](../charts/ceph-csi-cephfs/README.md)

## Read Affinity using crush locations for CephFS subvolumes

Ceph CSI supports mounting CephFS subvolumes with kernel mount options
`"read_from_replica=localize,crush_location=type1:value1|type2:value2"` to
allow serving reads from the most local OSD (according to OSD locations as
defined in the CRUSH map).

This can be enabled by adding labels to Kubernetes nodes like
`"topology.io/region=east"` and `"topology.io/zone=east-zone1"` and
passing command line arguments `"--enable-read-affinity=true"` and
`"--crush-location-labels=topology.io/zone,topology.io/region"` to Ceph CSI
CephFS daemonset pod "csi-cephfsplugin" container, resulting in Ceph CSI adding
`"--options read_from_replica=localize,crush_location=zone:east-zone1|region:east"`
kernel mount options during cephfs mount operation.
If enabled, this option will be added to all CephFS subvolumes mapped by Ceph CSI.
Well known labels can be found
[here](https://kubernetes.io/docs/reference/labels-annotations-taints/).

>Note: Label values will have all its dots `"."` normalized with dashes `"-"`
in order for it to work with ceph CRUSH map.

## CephFS Volume Encryption

Requires fscrypt support in the Linux kernel and Ceph.

Key management is compatible with the
[fscrypt](https://github.com/google/fscrypt) userspace tool. See the
design doc [Ceph Filesystem fscrypt
Support](design/proposals/cephfs-fscrypt.md) for details.

In general the KMS configuration is the same as for RBD encryption and
can even be shared.

However, not all KMS are supported in order to be compatible with
[fscrypt](https://github.com/google/fscrypt). In general KMS that
either store secrets to use directly (Vault), or allow access to the
plain password (Kubernetes Secrets) work.

## CephFS PVC Provisioning

Requires subvolumegroup to be created before provisioning the PVC.
If the subvolumegroup provided in `ceph-csi-config` ConfigMap is missing
in the ceph cluster, the PVC creation will fail and will stay in `Pending` state.
