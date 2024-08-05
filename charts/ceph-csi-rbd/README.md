# ceph-csi-rbd

The ceph-csi-rbd chart adds rbd volume support to your cluster.

## Install from release repo

Add chart repository to install helm charts from it

```console
helm repo add ceph-csi https://ceph.github.io/csi-charts
```

## Install from local Chart

we need to enter into the directory where all charts are present

```console
cd charts
```

**Note:** charts directory is present in root of the ceph-csi project

### Install chart

To install the Chart into your Kubernetes cluster

- For helm 2.x

    ```bash
    helm install --namespace "ceph-csi-rbd" --name "ceph-csi-rbd" ceph-csi/ceph-csi-rbd
    ```

- For helm 3.x

    Create the namespace where Helm should install the components with

    ```bash
    kubectl create namespace "ceph-csi-rbd"
    ```

    Run the installation

    ```bash
    helm install --namespace "ceph-csi-rbd" "ceph-csi-rbd" ceph-csi/ceph-csi-rbd
    ```

After installation succeeds, you can get a status of Chart

```bash
helm status --namespace "ceph-csi-rbd" "ceph-csi-rbd"
```

### Upgrade Chart

If you want to upgrade your Chart, use the following commands.

```bash
helm repo update ceph-csi
helm upgrade --namespace ceph-csi-rbd ceph-csi-rbd ceph-csi/ceph-csi-rbd
```

For upgrading to a specific version, provide the flag `--version` and the
version.

**Do not forget to include your values**, if they differ from the default values.
We recommend not to use `--reuse-values` in case there are new defaults AND
compare your currently used values with the new default values.

#### Known Issues Upgrading

- When upgrading to version >=3.7.0, you might encounter an error that the
  RBD CSI Driver cannot be updated. Please refer to
  [issue](https://github.com/ceph/ceph-csi/issues/3397) for more details.
  This is due to the CSIDriver resource not being updatable. To work around this
  you can delete the CSIDriver object by running:

  ```bash
  kubectl delete csidriver rbd.csi.ceph.com
  ```

  Then rerun your `helm upgrade` command.

### Delete Chart

If you want to delete your Chart, use this command

- For helm 2.x

    ```bash
    helm delete --purge "ceph-csi-rbd"
    ```

- For helm 3.x

    ```bash
    helm uninstall "ceph-csi-rbd" --namespace "ceph-csi-rbd"
    ```

If you want to delete the namespace, use this command

```bash
kubectl delete namespace ceph-csi-rbd
```

### Configuration

The following table lists the configurable parameters of the ceph-csi-cephfs
charts and their default values.

| Parameter                                      | Description                                                                                                                                          | Default                                            |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| `rbac.create`                                  | Specifies whether RBAC resources should be created                                                                                                   | `true`                                             |
| `serviceAccounts.nodeplugin.create`            | Specifies whether a nodeplugin ServiceAccount should be created                                                                                      | `true`                                             |
| `serviceAccounts.nodeplugin.name`              | The name of the nodeplugin ServiceAccount to use. If not set and create is true, a name is generated using the fullname                              | ""                                                 |
| `serviceAccounts.provisioner.create`           | Specifies whether a provisioner ServiceAccount should be created                                                                                     | `true`                                             |
| `serviceAccounts.provisioner.name`             | The name of the provisioner ServiceAccount to use. If not set and create is true, a name is generated using the fullname                             | ""                                                 |
| `csiConfig`                                    | Configuration for the CSI to connect to the cluster                                                                                                  | []                                                 |
| `csiMapping`                                   | Configuration details of clusterID,PoolID,FscID mapping                                                                                              | []                                                 |
| `encryptionKMSConfig`                          | Configuration for the encryption KMS                                                                                                                 | `{}`                                               |
| `commonLabels`                                 | Labels to apply to all resources                                                                  | `{}`                                                  |
| `logLevel`                                     | Set logging level for csi containers. Supported values from 0 to 5. 0 for general useful logs, 5 for trace level verbosity.                          | `5`                                                |
| `sidecarLogLevel`                              | Set logging level for csi sidecar containers. Supported values from 0 to 5. 0 for general useful logs, 5 for trace level verbosity.                  | `1`                                                |
| `nodeplugin.name`                              | Specifies the nodeplugins name                                                                                                                       | `nodeplugin`                                       |
| `nodeplugin.updateStrategy`                    | Specifies the update Strategy. If you are using ceph-fuse client set this value to OnDelete                                                          | `RollingUpdate`                                    |
| `nodeplugin.priorityClassName`                 | Set user created priorityclassName for csi plugin pods. default is system-node-critical which is highest priority                                    | `system-node-critical`                             |
| `nodeplugin.imagePullSecrets`                | Specifies imagePullSecrets for containers                                                                                                        | `[]`                                            |
| `nodeplugin.profiling.enabled`                 | Specifies whether profiling should be enabled                                                                                                        | `false`                                            |
| `nodeplugin.registrar.image.repository`        | Node Registrar image repository URL                                                                                                                  | `registry.k8s.io/sig-storage/csi-node-driver-registrar` |
| `nodeplugin.registrar.image.tag`               | Image tag                                                                                                                                            | `v2.11.1`                                           |
| `nodeplugin.registrar.image.pullPolicy`        | Image pull policy                                                                                                                                    | `IfNotPresent`                                     |
| `nodeplugin.plugin.image.repository`           | Nodeplugin image repository URL                                                                                                                      | `quay.io/cephcsi/cephcsi`                          |
| `nodeplugin.plugin.image.tag`                  | Image tag                                                                                                                                            | `canary`                                           |
| `nodeplugin.plugin.image.pullPolicy`           | Image pull policy                                                                                                                                    | `IfNotPresent`                                     |
| `nodeplugin.podSecurityContext`                | Specifies pod-level security context.                                                                                                                | `{}`                                               |
| `nodeplugin.nodeSelector`                      | Kubernetes `nodeSelector` to add to the Daemonset                                                                                                    | `{}`                                               |
| `nodeplugin.tolerations`                       | List of Kubernetes `tolerations` to add to the Daemonset                                                                                             | `{}`                                               |
| `provisioner.name`                             | Specifies the name of provisioner                                                                                                                    | `provisioner`                                      |
| `provisioner.replicaCount`                     | Specifies the replicaCount                                                                                                                           | `3`                                                |
| `provisioner.defaultFSType`                    | Specifies the default Fstype                                                                                                                         | `ext4`                                             |
| `provisioner.deployController`                 | It enables or disables the deployment of controller which generates the OMAP data if it is not present                                               | `true`                                             |
| `provisioner.hardMaxCloneDepth`                | Hard limit for maximum number of nested volume clones that are taken before a flatten occurs                                                         | `8`                                                |
| `provisioner.softMaxCloneDepth`                | Soft limit for maximum number of nested volume clones that are taken before a flatten occurs                                                         | `4`                                                |
| `provisioner.maxSnapshotsOnImage`              | Maximum number of snapshots allowed on rbd image without flattening                                                                                  | `450`                                              |
| `provisioner.minSnapshotsOnImage`              | Minimum number of snapshots allowed on rbd image to trigger flattening                                                                               | `250`                                              |
| `provisioner.skipForceFlatten`                 | Skip image flattening if kernel support mapping of rbd images which has the deep-flatten feature                                                     | `false`                                            |
| `provisioner.timeout`                          | GRPC timeout for waiting for creation or deletion of a volume                                                                                        | `60s`                                              |
| `provisioner.clustername`                      | Cluster name to set on the RBD image                                                                                                                 | ""                                                 |
| `provisioner.setmetadata`                      | Set metadata on volume                                                                                                                               | `true`                                             |
| `provisioner.priorityClassName`                | Set user created priorityclassName for csi provisioner pods. Default is `system-cluster-critical` which is less priority than `system-node-critical` | `system-cluster-critical`                          |
| `provisioner.enableHostNetwork`                | Specifies whether hostNetwork is enabled for provisioner pod.                                                                                        | `false`                                            |
| `provisioner.imagePullSecrets`                | Specifies imagePullSecrets for containers                                                                                                        | `[]`                                            |
| `provisioner.profiling.enabled`                | Specifies whether profiling should be enabled                                                                                                        | `false`                                            |
| `provisioner.provisioner.image.repository`     | Specifies the csi-provisioner image repository URL                                                                                                   | `registry.k8s.io/sig-storage/csi-provisioner`           |
| `provisioner.provisioner.image.tag`            | Specifies image tag                                                                                                                                  | `v5.0.1`                                           |
| `provisioner.provisioner.image.pullPolicy`     | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.provisioner.image.extraArgs`      | Specifies extra arguments for the provisioner sidecar                                                                                                                                | `[]`                                     |
| `provisioner.podSecurityContext`               | Specifies pod-level security context.                                                                                                                | `{}`                                               |
| `provisioner.snapshotter.args.enableVolumeGroupSnapshots` | enables the creation of volume group snapshots | `false` |
| `provisioner.attacher.image.repository`        | Specifies the csi-attacher image repository URL                                                                                                      | `registry.k8s.io/sig-storage/csi-attacher`              |
| `provisioner.attacher.image.tag`               | Specifies image tag                                                                                                                                  | `v4.5.`                                           |
| `provisioner.attacher.image.pullPolicy`        | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.attacher.image.extraArgs`         | Specifies extra arguments for the attacher sidecar                                                                                                                                | `[]`                                     |
| `provisioner.attacher.name`                    | Specifies the name of csi-attacher sidecar                                                                                                           | `attacher`                                         |
| `provisioner.attacher.enabled`                 | Specifies whether attacher sidecar is enabled                                                                                                        | `true`                                             |
| `provisioner.resizer.image.repository`         | Specifies the csi-resizer image repository URL                                                                                                       | `registry.k8s.io/sig-storage/csi-resizer`               |
| `provisioner.resizer.image.tag`                | Specifies image tag                                                                                                                                  | `v1.11.1`                                           |
| `provisioner.resizer.image.pullPolicy`         | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.resizer.image.extraArgs`          | Specifies extra arguments for the resizer sidecar                                                                                                                                | `[]`                                     |
| `provisioner.resizer.name`                     | Specifies the name of csi-resizer sidecar                                                                                                            | `resizer`                                          |
| `provisioner.resizer.enabled`                  | Specifies whether resizer sidecar is enabled                                                                                                         | `true`                                             |
| `provisioner.snapshotter.image.repository`     | Specifies the csi-snapshotter image repository URL                                                                                                   | `registry.k8s.io/sig-storage/csi-snapshotter`          |
| `provisioner.snapshotter.image.tag`            | Specifies image tag                                                                                                                                  | `v8.0.1`                                           |
| `provisioner.snapshotter.image.pullPolicy`     | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.snapshotter.image.extraArgs`      | Specifies extra arguments for the snapshotter sidecar                                                                                                                                | `[]`                                     |
| `provisioner.nodeSelector`                     | Specifies the node selector for provisioner deployment                                                                                               | `{}`                                               |
| `provisioner.tolerations`                      | Specifies the tolerations for provisioner deployment                                                                                                 | `{}`                                               |
| `provisioner.affinity`                         | Specifies the affinity for provisioner deployment                                                                                                    | `{}`                                               |
| `topology.enabled`                             | Specifies whether topology based provisioning support should be exposed by CSI                                                                       | `false`                                            |
| `topology.domainLabels`                        | DomainLabels define which node labels to use as domains for CSI nodeplugins to advertise their domains                                               | `{}`                                               |
| `readAffinity.enabled` | Enable read affinity for RBD volumes. Recommended to set to true if running kernel 5.8 or newer. | `false` |
| `readAffinity.crushLocationLabels` | Define which node labels to use as CRUSH location. This should correspond to the values set in the CRUSH map. For more information, click [here](https://github.com/ceph/ceph-csi/blob/v3.9.0/docs/deploy-rbd.md#read-affinity-using-crush-locations-for-rbd-volumes)| `[]` |
| `provisionerSocketFile`                        | The filename of the provisioner socket                                                                                                               | `csi-provisioner.sock`                             |
| `pluginSocketFile`                             | The filename of the plugin socket                                                                                                                    | `csi.sock`                                         |
| `kubeletDir`                                   | kubelet working directory                                                                                                                            | `/var/lib/kubelet`                                 |
| `cephLogDirHostPath`                           | Host path location for ceph client processes logging, ex: rbd-nbd                                                                                    | `/var/log/ceph`                                    |
| `driverName`                                   | Name of the csi-driver                                                                                                                               | `rbd.csi.ceph.com`                                 |
| `configMapName`                                | Name of the configmap which contains cluster configuration                                                                                           | `ceph-csi-config`                                  |
| `externallyManagedConfigmap`                   | Specifies the use of an externally provided configmap                                                                                                | `false`                                            |
| `cephConfConfigMapName`                        | Name of the configmap which contains ceph.conf configuration                                                                                           | `ceph-config`                                  |
| `kmsConfigMapName`                             | Name of the configmap used for encryption kms configuration                                                                                          | `ceph-csi-encryption-kms-config`                   |
| `storageClass.create`                          | Specifies whether the StorageClass should be created                                                                                                 | `false`                                            |
| `storageClass.name`                            | Specifies the rbd StorageClass name                                                                                                                  | `csi-rbd-sc`                                       |
| `storageClass.annotations`                     | Specifies the annotations for the rbd StorageClass                                                                                                   | `[]`                                               |
| `storageClass.clusterID`                       | String representing a Ceph cluster to provision storage from                                                                                         | `<cluster-ID>`                                     |
| `storageClass.dataPool`                        | Specifies the erasure coded pool                                                                                                                     | `""`                                               |
| `storageClass.pool`                            | Ceph pool into which the RBD image shall be created                                                                                                  | `replicapool`                                      |
| `storageclass.imageFeatures`                   | Specifies RBD image features                                                                                                                         | `layering`                                         |
| `storageclass.tryOtherMounters`                | Specifies whether to try other mounters in case if the current mounter fails to mount the rbd image for any reason                                   | `false`                                            |
| `storageclass.mkfsOptions`                | Options to pass to the `mkfs` command while creating the filesystem on the RBD device                                   | `""`                                            |
| `storageClass.mounter`                         | Specifies RBD mounter                                                                                                                                | `""`                                               |
| `storageClass.cephLogDir`                      | ceph client log location, it is the target bindmount path used inside container                                                                      | `"/var/log/ceph"`                                  |
| `storageClass.cephLogStrategy`                 | ceph client log strategy, available options `remove` or `compress` or `preserve`                                                                     | `"remove"`                                         |
| `storageClass.volumeNamePrefix`                | Prefix to use for naming RBD images                                                                                                                  | `""`                                               |
| `storageClass.encrypted`                       | Specifies whether volume should be encrypted. Set it to true if you want to enable encryption                                                        | `""`                                               |
| `storageClass.encryptionKMSID`                 | Specifies the encryption kms id                                                                                                                      | `""`                                               |
| `storageClass.topologyConstrainedPools`        | Add topology constrained pools configuration, if topology based pools are setup, and topology constrained provisioning is required                   | `[]`                                               |
| `storageClass.mapOptions`                      | Specifies comma-separated list of map options                                                                                                        | `""`                                               |
| `storageClass.unmapOptions`                    | Specifies comma-separated list of unmap options                                                                                                      | `""`                                               |
| `storageClass.stripeUnit`                      | Specifies the stripe unit in bytes                                                                                                                   | `""`                                               |
| `storageClass.stripeCount`                     | Specifies the number of objects to stripe over before looping                                                                                        | `""`                                               |
| `storageClass.objectSize`                      | Specifies the object size in bytes                                                                                                                   | `""`                                               |
| `storageClass.provisionerSecret`               | The secrets have to contain user and/or Ceph admin credentials.                                                                                      | `csi-rbd-secret`                                   |
| `storageClass.provisionerSecretNamespace`      | Specifies the provisioner secret namespace                                                                                                           | `""`                                               |
| `storageClass.controllerExpandSecret`          | Specifies the controller expand secret name                                                                                                          | `csi-rbd-secret`                                   |
| `storageClass.controllerExpandSecretNamespace` | Specifies the controller expand secret namespace                                                                                                     | `""`                                               |
| `storageClass.nodeStageSecret`                 | Specifies the node stage secret name                                                                                                                 | `csi-rbd-secret`                                   |
| `storageClass.nodeStageSecretNamespace`        | Specifies the node stage secret namespace                                                                                                            | `""`                                               |
| `storageClass.fstype`                          | Specify the filesystem type of the volume                                                                                                            | `ext4`                                             |
| `storageClass.reclaimPolicy`                   | Specifies the reclaim policy of the StorageClass                                                                                                     | `Delete`                                           |
| `storageClass.allowVolumeExpansion`            | Specifies whether volume expansion should be allowed                                                                                                 | `true`                                             |
| `storageClass.mountOptions`                    | Specifies the mount options for storageClass                                                                                                         | `[]`                                               |
| `secret.create`                                | Specifies whether the secret should be created                                                                                                       | `false`                                            |
| `secret.name`                                  | Specifies the rbd secret name                                                                                                                        | `csi-rbd-secret`                                   |
| `secret.userID`                                | Specifies the user ID of the rbd secret                                                                                                              | `<plaintext ID>`                                   |
| `secret.userKey`                               | Specifies the key that corresponds to the userID                                                                                                     | `<Ceph auth key corresponding to ID above>`        |
| `secret.encryptionPassphrase`                  | Specifies the encryption passphrase of the secret                                                                                                    | `test_passphrase`                                  |
| `selinuxMount`                                | Mount the host /etc/selinux inside pods to support selinux-enabled filesystems                                                                                                      | `true`                                            |
| `CSIDriver.fsGroupPolicy` | Specifies the fsGroupPolicy for the CSI driver object | `File` |
| `CSIDriver.seLinuxMount` | Specify for efficient SELinux volume relabeling | `true` |
| `instanceID`                                   | Unique ID distinguishing this instance of Ceph CSI among other instances, when sharing Ceph clusters across CSI instances for provisioning. | ` ` |

### Command Line

You can pass the settings with helm command line parameters.
Specify each parameter using the --set key=value argument to helm install.
For Example:

```bash
helm install --set configMapName=ceph-csi-config
```
