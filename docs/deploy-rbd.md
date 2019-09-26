
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

| Option                | Default value         | Description                                                                                                                                                                                                                                                                          |
| --------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--endpoint`          | `unix://tmp/csi.sock` | CSI endpoint, must be a UNIX socket                                                                                                                                                                                                                                                  |
| `--drivername`        | `rbd.csi.ceph.com`    | Name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)                                                                                                                                                                                   |
| `--nodeid`            | _empty_               | This node's ID                                                                                                                                                                                                                                                                       |
| `--type`              | _empty_               | Driver type `[rbd | cephfs]` If the driver type is set to  `rbd` it will act as a `rbd plugin` or if it's set to `cephfs` will act as a `cephfs plugin`                                                                                                                              |
| `--containerized`     | false                 | Whether running in containerized mode( This flag is deprecated)                                                                                                                                                                                                                      |
| `--instanceid`        | "default"             | Unique ID distinguishing this instance of Ceph CSI among other instances, when sharing Ceph clusters across CSI instances for provisioning                                                                                                                                           |
| `--metadatastorage`   | _empty_               | Points to where legacy (1.0.0 or older plugin versions) metadata about provisioned volumes are kept, as file or in as k8s configmap (`node` or `k8s_configmap` respectively)                                                                                                         |
| `--pidlimit`          | _0_                   | Configure the PID limit in cgroups. The container runtime can restrict the number of processes/tasks which can cause problems while provisioning (or deleting) a large number of volumes. A value of `-1` configures the limit to the maximum, `0` does not configure limits at all. |
| `--metricsport`       | `8080`                | TCP port for liveness/grpc metrics requests                                                                                                                                                                                                                                          |
| `--metricspath`       | `"/metrics"`          | Path of prometheus endpoint where metrics will be available                                                                                                                                                                                                                          |
| `--enablegrpcmetrics` | `false`               | Enable grpc metrics collection  and start prometheus server                                                                                                                                                                                                                          |
| `--polltime`          | `"60s"`               | Time interval in between each poll                                                                                                                                                                                                                                                   |
| `--timeout`           | `"3s"`                | Probe timeout in seconds                                                                                                                                                                                                                                                             |
| `--histogramoption`   | `0.5,2,6`             | Histogram option for grpc metrics, should be comma separated value (ex:= "0.5,2,6" where start=0.5 factor=2, count=6)                                                                                                                                                                |

**Available volume parameters:**

| Parameter                                                                                           | Required             | Description                                                                                                                                                                                                             |
| --------------------------------------------------------------------------------------------------- | -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `clusterID`                                                                                         | yes                  | String representing a Ceph cluster, must be unique across all Ceph clusters in use for provisioning, cannot be greater than 36 bytes in length, and should remain immutable for the lifetime of the Ceph cluster in use |
| `pool`                                                                                              | yes                  | Ceph pool into which the RBD image shall be created                                                                                                                                                                     |
| `dataPool`                                                                                          | no                   | Ceph pool used for the data of the RBD images.                                                                                                                                                                          |
| `imageFeatures`                                                                                     | no                   | RBD image features. CSI RBD currently supports only `layering` feature. See [man pages](http://docs.ceph.com/docs/mimic/man/8/rbd/#cmdoption-rbd-image-feature)                                                         |
| `csi.storage.k8s.io/provisioner-secret-name`, `csi.storage.k8s.io/node-stage-secret-name`           | yes (for Kubernetes) | name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value                                                                                                     |
| `csi.storage.k8s.io/provisioner-secret-namespace`, `csi.storage.k8s.io/node-stage-secret-namespace` | yes (for Kubernetes) | namespaces of the above Secret objects                                                                                                                                                                                  |
| `mounter`                                                                                           | no                   | if set to `rbd-nbd`, use `rbd-nbd` on nodes that have `rbd-nbd` and `nbd` kernel modules to map rbd images                                                                                                              |

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

Requires Kubernetes 1.13+

if your cluster version is 1.13.x please use [rbd v1.13
templates](../deploy/rbd/kubernetes/v1.13) or else use [rbd v1.14+
templates](../deploy/rbd/kubernetes/v1.14+)

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

**Deploy ConfigMap for CSI plugins:**

```bash
kubectl create -f csi-config-map.yaml
```

The config map deploys an empty CSI configuration that is mounted as a volume
within the Ceph CSI plugin pods. To add a specific Ceph clusters configuration
details, refer to [Creating CSI configuration for RBD based
provisioning](../examples/README.md#creating-csi-configuration-for-rbd-based-provisioning)
for more information.

**Deploy CSI sidecar containers:**

```bash
kubectl create -f csi-rbdplugin-provisioner.yaml
```

Deploys stateful set of provision which includes external-provisioner
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
