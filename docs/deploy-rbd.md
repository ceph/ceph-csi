# CSI RBD Plugin

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

| Option                   | Default value         | Description                                                                                                                                                                                                                                                                          |
| ------------------------ | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--endpoint`             | `unix://tmp/csi.sock` | CSI endpoint, must be a UNIX socket                                                                                                                                                                                                                                                  |
| `--drivername`           | `rbd.csi.ceph.com`    | Name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)                                                                                                                                                                                   |
| `--nodeid`               | _empty_               | This node's ID                                                                                                                                                                                                                                                                       |
| `--type`                 | _empty_               | Driver type: `[rbd/cephfs]`. If the driver type is set to  `rbd` it will act as a `rbd plugin` or if it's set to `cephfs` will act as a `cephfs plugin`                                                                                                                              |
| `--instanceid`           | "default"             | Unique ID distinguishing this instance of Ceph CSI among other instances, when sharing Ceph clusters across CSI instances for provisioning                                                                                                                                           |
| `--pidlimit`             | _0_                   | Configure the PID limit in cgroups. The container runtime can restrict the number of processes/tasks which can cause problems while provisioning (or deleting) a large number of volumes. A value of `-1` configures the limit to the maximum, `0` does not configure limits at all. |
| `--metricsport`          | `8080`                | TCP port for liveness metrics requests                                                                                                                                                                                                                                               |
| `--metricspath`          | `"/metrics"`          | Path of prometheus endpoint where metrics will be available                                                                                                                                                                                                                          |
| `--enablegrpcmetrics`    | `false`               | [Deprecated] Enable grpc metrics collection  and start prometheus server                                                                                                                                                                                                             |
| `--polltime`             | `"60s"`               | Time interval in between each poll                                                                                                                                                                                                                                                   |
| `--timeout`              | `"3s"`                | Probe timeout in seconds                                                                                                                                                                                                                                                             |
| `--histogramoption`      | `0.5,2,6`             | [Deprecated] Histogram option for grpc metrics, should be comma separated value (ex:= "0.5,2,6" where start=0.5 factor=2, count=6)                                                                                                                                                   |
| `--domainlabels`         | _empty_               | Kubernetes node labels to use as CSI domain labels for topology aware provisioning, should be a comma separated value (ex:= "failure-domain/region,failure-domain/zone")                                                                                                             |
| `--rbdhardmaxclonedepth` | `8`                   | Hard limit for maximum number of nested volume clones that are taken before a flatten occurs                                                                                                                                                                                         |
| `--rbdsoftmaxclonedepth` | `4`                   | Soft limit for maximum number of nested volume clones that are taken before a flatten occurs                                                                                                                                                                                         |
| `--skipforceflatten`     | `false`               | skip image flattening on kernel < 5.2 which support mapping of rbd images which has the deep-flatten feature                                                                                                                                                                         |
| `--maxsnapshotsonimage`  | `450`                 | Maximum number of snapshots allowed on rbd image without flattening                                                                                                                                                                                                                  |

**Available volume parameters:**

| Parameter                                                                                           | Required             | Description                                                                                                                                                                                                             |
| --------------------------------------------------------------------------------------------------- | -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `clusterID`                                                                                         | yes                  | String representing a Ceph cluster, must be unique across all Ceph clusters in use for provisioning, cannot be greater than 36 bytes in length, and should remain immutable for the lifetime of the Ceph cluster in use |
| `pool`                                                                                              | yes                  | Ceph pool into which the RBD image shall be created                                                                                                                                                                     |
| `dataPool`                                                                                          | no                   | Ceph pool used for the data of the RBD images.                                                                                                                                                                          |
| `volumeNamePrefix`                                                                                  | no                   | Prefix to use for naming RBD images (defaults to `csi-vol-`).                                                                                                                                                           |
| `snapshotNamePrefix`                                                                                | no                   | Prefix to use for naming RBD snapshot images (defaults to `csi-snap-`).                                                                                                                                                 |
| `imageFeatures`                                                                                     | yes                  | RBD image features. CSI RBD currently supports only `layering` feature. See [man pages](http://docs.ceph.com/docs/master/man/8/rbd/#cmdoption-rbd-image-feature) Note that the required support for [object-map and fast-diff were added in 5.3 and journaling does not have KRBD support yet](https://docs.ceph.com/en/latest/rbd/rbd-config-ref/#image-features) which is why they are unsupported here. deep-flatten is added for cloned images. |
| `mapOptions`                                                                                        | no                   | Map options to use when mapping rbd image. See [krbd](https://docs.ceph.com/docs/master/man/8/rbd/#kernel-rbd-krbd-options) and [nbd](https://docs.ceph.com/docs/master/man/8/rbd-nbd/#options) options.                |
| `unmapOptions`                                                                                      | no                   | Unmap options to use when unmapping rbd image. See [krbd](https://docs.ceph.com/docs/master/man/8/rbd/#kernel-rbd-krbd-options) and [nbd](https://docs.ceph.com/docs/master/man/8/rbd-nbd/#options) options.            |
| `csi.storage.k8s.io/provisioner-secret-name`, `csi.storage.k8s.io/node-stage-secret-name`           | yes (for Kubernetes) | name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value                                                                                                     |
| `csi.storage.k8s.io/provisioner-secret-namespace`, `csi.storage.k8s.io/node-stage-secret-namespace` | yes (for Kubernetes) | namespaces of the above Secret objects                                                                                                                                                                                  |
| `mounter`                                                                                           | no                   | if set to `rbd-nbd`, use `rbd-nbd` on nodes that have `rbd-nbd` and `nbd` kernel modules to map rbd images                                                                                                              |
| `encrypted`                                                                                         | no                   | disabled by default, use `"true"` to enable LUKS encryption on PVC and `"false"` to disable it. **Do not change for existing storageclasses**                                                                           |
| `encryptionKMSID`                                                                                   | no                   | required if encryption is enabled and a kms is used to store passphrases                                                                                                                                                |
| `thickProvision`                                                                                    | no                   | if set to `"true"`, newly created RBD images will be completely allocated by writing zeros to it                                                                                                                        |

**NOTE:** An accompanying CSI configuration file, needs to be provided to the
running pods. Refer to [Creating CSI configuration](../examples/README.md#creating-csi-configuration)
for more information.

**NOTE:** A suggested way to populate and retain uniqueness of the clusterID is
to use the output of `ceph fsid` of the Ceph cluster to be used for
provisioning.

**Required secrets:**

User credentials, with required access to the pool being used in the storage class,
is required for provisioning new RBD images.

## Deployment with Kubernetes

Requires Kubernetes 1.14+

Use the [rbd templates](../deploy/rbd/kubernetes)

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged`
flag must be set to true for both the API server and the kubelet). Moreover, as
stated in the [mount propagation
docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation),
the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in `deploy/rbd/kubernetes`.

**Deploy RBACs for sidecar containers and node plugins:**

```bash
kubectl create -f csi-provisioner-rbac.yaml
kubectl create -f csi-nodeplugin-rbac.yaml
```

Those manifests deploy service accounts, cluster roles and cluster role
bindings. These are shared for both RBD and CephFS CSI plugins, as they require
the same permissions.

**Deploy PodSecurityPolicy resources for sidecar containers and node plugins:**

**NOTE:** These manifests are required only if [PodSecurityPolicy](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#podsecuritypolicy)
admission controller is active on your cluster.

```bash
kubectl create -f csi-provisioner-psp.yaml
kubectl create -f csi-nodeplugin-psp.yaml
```

**Deploy ConfigMap for CSI plugins:**

```bash
kubectl create -f csi-config-map.yaml
```

The configmap deploys an empty CSI configuration that is mounted as a volume
within the Ceph CSI plugin pods. To add a specific Ceph clusters configuration
details, refer to [Creating CSI configuration for RBD based
provisioning](../examples/README.md#creating-csi-configuration-for-rbd-based-provisioning)
for more information.

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
instructions [provided](../examples/README.md#deploying-the-storage-class) to
test the deployment further.

## Deployment with Helm

The same requirements from the Kubernetes section apply here, i.e. Kubernetes
version, privileged flag and shared mounts.

The Helm chart is located in `charts/ceph-csi-rbd`.

**Deploy Helm Chart:**

[See the Helm chart readme for installation instructions.](../charts/ceph-csi-rbd/README.md)

## Encryption for RBD volumes

> Enabling encryption on volumes created without encryption is **not supported**
>
> Enabling encryption for storage class that has PVs created without encryption
> is **not supported**

Volumes provisioned with Ceph RBD do not have encryption by default. It is
possible to encrypt them with ceph-csi by using LUKS encryption.

### Life-cycle for encrypted volumes

**Create volume**:

* create volume request received
* volume requested to be created in Ceph
* new passphrase is generated and stored in selected KMS if KMS is in use
* encrypted state "encryptionPrepared" is saved in image-meta in Ceph

**Attach volume**:

* attach volume request received
* volume is attached to provisioner container
* on first time attachment
  (no file system on the attached device, checked with blkid)
  * passphrase is retrieved from selected KMS if KMS is in use
  * device is encrypted with LUKS using a passphrase from K8s Secret or KMS
  * image-meta updated to "encrypted" in Ceph
* passphrase is retrieved from selected KMS if KMS is in use
* device is open and device path is changed to use a mapper device
* mapper device is used instead of original one with usual workflow

**Detach volume**:

* mapper device closed and device path changed to original volume path
* volume is detached as usual
* passphrase removed from KMS if needed (with failures ignored)

### Encryption configuration

To encrypt rbd volumes with LUKS you need to set encryption passphrase in
secrets under `encryptionPassphrase` key and switch `encrypted` option in
StorageClass to `"true"`. This is not supported for storage classes that already
have PVs provisioned. The `node-stage-secret-name` and the `provisioner-secret-name`
should carry this key and value for encryption to work.

To use different passphrase you need to have different storage classes and point
to a different K8s secrets `csi.storage.k8s.io/node-stage-secret-name`
and `csi.storage.k8s.io/provisioner-secret-name` which carry new passphrase value
for `encryptionPassphrase` key in these secrets.

### Encryption KMS configuration

To further improve security robustness it is possible to use unique passphrases
generated for each volume and stored in a Key Management System (KMS). Currently
HashiCorp Vault is the only KMS supported.

There are two options to use Hashicorp Vault as a KMS:

1. with Kubernetes ServiceAccount
1. with a Vault Token per Tenant (a Kubernetes Namespace)

To use Vault as KMS set `encryptionKMSID` to a unique identifier for Vault
configuration. You will also need to create vault configuration similar to the
[example](../examples/kms/vault/kms-config.yaml) and use same
`encryptionKMSID`.

To use the Kubernetes ServiceAccount to access Vault, the configuration must
include `encryptionKMSType: "vault"`. If Tenants are expected to place their
Vault Token in a Kubernetes Secret in their Namespace, set `encryptionKMSType:
"vaulttokens"`.

In order for ceph-csi to be able to access the configuration you will need to
have it mounted to csi-rbdplugin containers in both daemonset (so kms client
can be instantiated to encrypt/decrypt volumes) and deployment pods (so kms
client can be instantiated to delete passphrase on volume delete)
`ceph-csi-encryption-kms-config` configmap.

> Note: kms configuration must be a map of string values only
> (`map[string]string`) so for numerical and boolean values make sure to put
> quotes around.

When the Tenants need to provide their own Vault Token, they will need to place
it in a Kubernetes Secret (by default) called `ceph-csi-kms-token`, where the
Vault Token is stored in the `token` key as shown in [the
example](../examples/kms/vault/tenant-token.yaml).

#### Configuring HashiCorp Vault with Kubernetes ServiceAccount

Using Vault as KMS you need to configure Kubernetes authentication method as
described in [official
documentation](https://www.vaultproject.io/docs/auth/kubernetes.html).

If token reviewer is used, you will need to configure service account for
that also like in
[example](../examples/kms/vault/csi-vaulttokenreview-rbac.yaml) to be able to
review jwt tokens.

Configure a role(s) for service accounts used for ceph-csi:

* provisioner service account (`rbd-csi-provisioner`) requires only **delete**
  permissions to delete passphrases on PVC delete
* nodeplugin service account (`rbd-csi-nodeplugin`) requires **create** and
  **read** permissions to save new keys and retrieve existing

### Encryption prerequisites

In order for encryption to work you need to make sure that `dm-crypt` kernel
module is enabled on the nodes running ceph-csi attachers.

If custom image is built for the rbd-plugin instance, make sure that it contains
`cryptsetup` tool installed to be able to use encryption.
