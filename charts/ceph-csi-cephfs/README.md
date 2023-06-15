# ceph-csi-cephfs

The ceph-csi-cephfs chart adds cephFS volume support to your cluster.

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

### Install Chart

To install the Chart into your Kubernetes cluster

- For helm 2.x

    ```bash
    helm install --namespace "ceph-csi-cephfs" --name "ceph-csi-cephfs" ceph-csi/ceph-csi-cephfs
    ```

- For helm 3.x

    Create the namespace where Helm should install the components with

    ```bash
    kubectl create namespace ceph-csi-cephfs
    ```

    Run the installation

    ```bash
    helm install --namespace "ceph-csi-cephfs" "ceph-csi-cephfs" ceph-csi/ceph-csi-cephfs
    ```

After installation succeeds, you can get a status of Chart

```bash
helm status "ceph-csi-cephfs"
```

### Upgrade Chart

If you want to upgrade your Chart, use the following commands.

```bash
helm repo update ceph-csi
helm upgrade --namespace ceph-csi-cephfs ceph-csi-cephfs ceph-csi/ceph-csi-cephfs
```

For upgrading to a specific version, provide the flag `--version` and the
version.

**Do not forget to include your values**, if they differ from the default values.
We recommend not to use `--reuse-values` in case there are new defaults AND
compare your currently used values with the new default values.

#### Known Issues Upgrading

- When upgrading to version >=3.7.0, you might encounter an error that the
  CephFS CSI Driver cannot be updated. Please refer to
  [issue](https://github.com/ceph/ceph-csi/issues/3397) for more details.
  This is due to the CSIDriver resource not being updatable. To work around this
  you can delete the CSIDriver object by running:

  ```bash
  kubectl delete csidriver cephfs.csi.ceph.com
  ```

  Then rerun your `helm upgrade` command.

### Delete Chart

If you want to delete your Chart, use this command

- For helm 2.x

    ```bash
    helm delete --purge "ceph-csi-cephfs"
    ```

- For helm 3.x

    ```bash
    helm uninstall "ceph-csi-cephfs" --namespace "ceph-csi-cephfs"
    ```

If you want to delete the namespace, use this command

```bash
kubectl delete namespace ceph-csi-cephfs
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
| `serviceAccounts.provisioner.name`             | The name of the provisioner ServiceAccount of provisioner to use. If not set and create is true, a name is generated using the fullname              | ""                                                 |
| `csiConfig`                                    | Configuration for the CSI to connect to the cluster                                                                                                  | []                                                 |
| `commonLabels`                                 | Labels to apply to all resources                                                               | `{}`                                                  |
| `logLevel`                                     | Set logging level for csi containers. Supported values from 0 to 5. 0 for general useful logs, 5 for trace level verbosity.                          | `5`                                                |
| `sidecarLogLevel`                              | Set logging level for csi sidecar containers. Supported values from 0 to 5. 0 for general useful logs, 5 for trace level verbosity.                  | `1`                                                |
| `nodeplugin.name`                              | Specifies the nodeplugin name                                                                                                                        | `nodeplugin`                                       |
| `nodeplugin.updateStrategy`                    | Specifies the update Strategy. If you are using ceph-fuse client set this value to OnDelete                                                          | `RollingUpdate`                                    |
| `nodeplugin.priorityClassName`                 | Set user created priorityclassName for csi plugin pods. default is system-node-critical which is highest priority                                    | `system-node-critical`                             |
| `nodeplugin.imagePullSecrets`                | Specifies imagePullSecrets for containers                                                                                                        | `[]`                                            |
| `nodeplugin.profiling.enabled`                 | Specifies whether profiling should be enabled                                                                                                        | `false`                                            |
| `nodeplugin.registrar.image.repository`        | Node-Registrar image repository URL                                                                                                                  | `registry.k8s.io/sig-storage/csi-node-driver-registrar` |
| `nodeplugin.registrar.image.tag`               | Image tag                                                                                                                                            | `v2.8.0`                                           |
| `nodeplugin.registrar.image.pullPolicy`        | Image pull policy                                                                                                                                    | `IfNotPresent`                                     |
| `nodeplugin.plugin.image.repository`           | Nodeplugin image repository URL                                                                                                                      | `quay.io/cephcsi/cephcsi`                          |
| `nodeplugin.plugin.image.tag`                  | Image tag                                                                                                                                            | `canary`                                           |
| `nodeplugin.plugin.image.pullPolicy`           | Image pull policy                                                                                                                                    | `IfNotPresent`                                     |
| `nodeplugin.nodeSelector`                      | Kubernetes `nodeSelector` to add to the Daemonset                                                                                                    | `{}`                                               |
| `nodeplugin.tolerations`                       | List of Kubernetes `tolerations` to add to the Daemonset                                                                                             | `{}`                                               |
| `nodeplugin.forcecephkernelclient`             | Set to true to enable Ceph Kernel clients on kernel < 4.17 which support quotas                                                                      | `true`                                             |
| `nodeplugin.kernelmountoptions`                | Comma separated string of mount options accepted by cephfs kernel mounter quotas                                                                      | `""`                                               |
| `nodeplugin.fusemountoptions`                  | Comma separated string of mount options accepted by ceph-fuse mounter quotas                                                                      | `""`                                               |
| `nodeplugin.podSecurityPolicy.enabled`         | If true, create & use [Pod Security Policy resources](https://kubernetes.io/docs/concepts/policy/pod-security-policy/).                              | `false`                                            |
| `provisioner.name`                             | Specifies the name of provisioner                                                                                                                    | `provisioner`                                      |
| `provisioner.replicaCount`                     | Specifies the replicaCount                                                                                                                           | `3`                                                |
| `provisioner.timeout`                          | GRPC timeout for waiting for creation or deletion of a volume                                                                                        | `60s`                                              |
| `provisioner.clustername`                      | Cluster name to set on the subvolume                                                                                                                 | ""                                                 |
| `provisioner.setmetadata`                      | Set metadata on volume                                                                                                                               | `true`                                             |
| `provisioner.priorityClassName`                | Set user created priorityclassName for csi provisioner pods. Default is `system-cluster-critical` which is less priority than `system-node-critical` | `system-cluster-critical`                          |
| `provisioner.enableHostNetwork`                | Specifies whether hostNetwork is enabled for provisioner pod.                                                                                        | `false`                                            |
| `provisioner.imagePullSecrets`                | Specifies imagePullSecrets for containers                                                                                                        | `[]`                                            |
| `provisioner.profiling.enabled`                | Specifies whether profiling should be enabled                                                                                                        | `false`                                            |
| `provisioner.provisioner.image.repository`     | Specifies the csi-provisioner image repository URL                                                                                                   | `registry.k8s.io/sig-storage/csi-provisioner`      |
| `provisioner.provisioner.image.tag`            | Specifies image tag                                                                                                                                  | `v3.5.0`                                           |
| `provisioner.provisioner.image.pullPolicy`     | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.provisioner.image.extraArgs`      | Specifies extra arguments for the provisioner sidecar                                                                                                                                | `[]`                                     |
| `provisioner.resizer.image.repository`         | Specifies the csi-resizer image repository URL                                                                                                       | `registry.k8s.io/sig-storage/csi-resizer`          |
| `provisioner.resizer.image.tag`                | Specifies image tag                                                                                                                                  | `v1.8.0`                                           |
| `provisioner.resizer.image.pullPolicy`         | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.resizer.image.extraArgs`          | Specifies extra arguments for the resizer sidecar                                                                                                                                | `[]`                                     |
| `provisioner.resizer.name`                     | Specifies the name of csi-resizer sidecar                                                                                                            | `resizer`                                          |
| `provisioner.resizer.enabled`                  | Specifies whether resizer sidecar is enabled                                                                                                         | `true`                                             |
| `provisioner.snapshotter.image.repository`     | Specifies the csi-snapshotter image repository URL                                                                                                   | `registry.k8s.io/sig-storage/csi-snapshotter`      |
| `provisioner.snapshotter.image.tag`            | Specifies image tag                                                                                                                                  | `v6.2.2`                                           |
| `provisioner.snapshotter.image.pullPolicy`     | Specifies pull policy                                                                                                                                | `IfNotPresent`                                     |
| `provisioner.snapshotter.image.extraArgs`      | Specifies extra arguments for the snapshotter sidecar                                                                                                                                | `[]`                                     |
| `provisioner.nodeSelector`                     | Specifies the node selector for provisioner deployment                                                                                               | `{}`                                               |
| `provisioner.tolerations`                      | Specifies the tolerations for provisioner deployment                                                                                                 | `{}`                                               |
| `provisioner.affinity`                         | Specifies the affinity for provisioner deployment                                                                                                    | `{}`                                               |
| `provisioner.podSecurityPolicy.enabled`        | Specifies whether podSecurityPolicy is enabled                                                                                                       | `false`                                            |
| `provisionerSocketFile`                        | The filename of the provisioner socket                                                                                                               | `csi-provisioner.sock`                             |
| `pluginSocketFile`                             | The filename of the plugin socket                                                                                                                    | `csi.sock`                                         |
| `kubeletDir`                                   | Kubelet working directory                                                                                                                            | `/var/lib/kubelet`                                 |
| `driverName`                                   | Name of the csi-driver                                                                                                                               | `cephfs.csi.ceph.com`                              |
| `configMapName`                                | Name of the configmap which contains cluster configuration                                                                                           | `ceph-csi-config`                                  |
| `externallyManagedConfigmap`                   | Specifies the use of an externally provided configmap                                                                                                | `false`                                            |
| `cephConfConfigMapName`                        | Name of the configmap which contains ceph.conf configuration                                                                                           | `ceph-config`                                  |
| `storageClass.create`                          | Specifies whether the StorageClass should be created                                                                                                 | `false`                                            |
| `storageClass.name`                            | Specifies the cephFS StorageClass name                                                                                                               | `csi-cephfs-sc`                                    |
| `storageClass.annotations`                     | Specifies the annotations for the cephFS storageClass                                                                                                | `[]`                                               |
| `storageClass.clusterID`                       | String representing a Ceph cluster to provision storage from                                                                                         | `<cluster-ID>`                                     |
| `storageClass.fsName`                          | CephFS filesystem name into which the volume shall be created                                                                                        | `myfs`                                             |
| `storageClass.pool`                            | Ceph pool into which volume data shall be stored                                                                                                     | `""`                                               |
| `storageClass.fuseMountOptions`                | Comma separated string of Ceph-fuse mount options                                                                                                    | `""`                                               |
| `storageclass.kernelMountOptions`              | Comma separated string of CephFS kernel mount options                                                                                                | `""`                                               |
| `storageClass.mounter`                         | The driver can use either ceph-fuse (fuse) or ceph kernelclient (kernel)                                                                             | `""`                                               |
| `storageClass.volumeNamePrefix`                | Prefix to use for naming subvolumes                                                                                                                  | `""`                                               |
| `storageClass.provisionerSecret`               | The secrets have to contain user and/or Ceph admin credentials.                                                                                      | `csi-cephfs-secret`                                |
| `storageClass.provisionerSecretNamespace`      | Specifies the provisioner secret namespace                                                                                                           | `""`                                               |
| `storageClass.controllerExpandSecret`          | Specifies the controller expand secret name                                                                                                          | `csi-cephfs-secret`                                |
| `storageClass.controllerExpandSecretNamespace` | Specifies the controller expand secret namespace                                                                                                     | `""`                                               |
| `storageClass.nodeStageSecret`                 | Specifies the node stage secret name                                                                                                                 | `csi-cephfs-secret`                                |
| `storageClass.nodeStageSecretNamespace`        | Specifies the node stage secret namespace                                                                                                            | `""`                                               |
| `storageClass.reclaimPolicy`                   | Specifies the reclaim policy of the StorageClass                                                                                                     | `Delete`                                           |
| `storageClass.allowVolumeExpansion`            | Specifies whether volume expansion should be allowed                                                                                                 | `true`                                             |
| `storageClass.mountOptions`                    | Specifies the mount options                                                                                                                          | `[]`                                               |
| `secret.create`                                | Specifies whether the secret should be created                                                                                                       | `false`                                            |
| `secret.name`                                  | Specifies the cephFS secret name                                                                                                                     | `csi-cephfs-secret`                                |
| `secret.adminID`                               | Specifies the admin ID of the cephFS secret                                                                                                          | `<plaintext ID>`                                   |
| `secret.adminKey`                              | Specifies the key that corresponds to the adminID                                                                                                    | `<Ceph auth key corresponding to ID above>`        |
| `selinuxMount`                                | Mount the host /etc/selinux inside pods to support selinux-enabled filesystems                                                                                                      | `true`                                            |

### Command Line

You can pass the settings with helm command line parameters.
Specify each parameter using the --set key=value argument to helm install.
For Example:

```bash
helm install --set configMapName=ceph-csi-config --set provisioner.podSecurityPolicy.enabled=true
```
