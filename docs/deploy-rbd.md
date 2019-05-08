# CSI RBD Plugin

The RBD CSI plugin is able to provision new RBD images and
attach and mount those to workloads.

## Building

CSI RBD plugin can be compiled in a form of a binary file or in a form of a
Docker image. When compiled as a binary file, the result is stored in
`_output/` directory with the name `cephcsi`. When compiled as an image, it's
stored in the local Docker image store with name `rbdplugin`.

Building binary:

```bash
make cephcsi
```

Building Docker image:

```bash
make image-rbdplugin
```

## Configuration

**Available command line arguments:**

Option | Default value | Description
------ | ------------- | -----------
`--endpoint` | `unix://tmp/csi.sock` | CSI endpoint, must be a UNIX socket
`--drivername` | `rbd.csi.ceph.com` | name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)
`--nodeid` | _empty_ | This node's ID
`--containerized` | true | Whether running in containerized mode
`--metadatastorage` | _empty_ | Whether should metadata be kept on node as file or in a k8s configmap (`node` or `k8s_configmap`)
`--configroot` | `/etc/csi-config` | Directory in which CSI specific Ceph cluster configurations are present, OR the value `k8s_objects` if present as kubernetes secrets"

**Available environmental variables:**

`HOST_ROOTFS`: rbdplugin searches `/proc` directory under the directory set by `HOST_ROOTFS`.

`KUBERNETES_CONFIG_PATH`: if you use `k8s_configmap` as metadata store, specify
the path of your k8s config file (if not specified, the plugin will assume
you're running it inside a k8s cluster and find the config itself).

`POD_NAMESPACE`: if you use `k8s_configmap` as metadata store,
`POD_NAMESPACE` is used to define in which namespace you want
the configmaps to be stored

**Available volume parameters:**

Parameter | Required | Description
--------- | -------- | -----------
`monitors` | one of `monitors`, `clusterID` or `monValueFromSecret` must be set | Comma separated list of Ceph monitors (e.g. `192.168.100.1:6789,192.168.100.2:6789,192.168.100.3:6789`)
`monValueFromSecret` | one of `monitors`, `clusterID` or and `monValueFromSecret` must be set | a string pointing the key in the credential secret, whose value is the mon. This is used for the case when the monitors' IP or hostnames are changed, the secret can be updated to pick up the new monitors.
`clusterID` | one of `monitors`, `clusterID` or `monValueFromSecret` must be set | String representing a Ceph cluster, must be unique across all Ceph clusters in use for provisioning, cannot be greater than 36 bytes in length, and should remain immutable for the lifetime of the Ceph cluster in use
`pool` | yes | Ceph pool into which the RBD image shall be created
`imageFormat` | no | RBD image format. Defaults to `2`. See [man pages](http://docs.ceph.com/docs/mimic/man/8/rbd/#cmdoption-rbd-image-format)
`imageFeatures` | no | RBD image features. Available for `imageFormat=2`. CSI RBD currently supports only `layering` feature. See [man pages](http://docs.ceph.com/docs/mimic/man/8/rbd/#cmdoption-rbd-image-feature)
`csi.storage.k8s.io/provisioner-secret-name`, `csi.storage.k8s.io/node-publish-secret-name` | for Kubernetes | name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value
`csi.storage.k8s.io/provisioner-secret-namespace`, `csi.storage.k8s.io/node-publish-secret-namespace` | for Kubernetes | namespaces of the above Secret objects
`mounter`| no | if set to `rbd-nbd`, use `rbd-nbd` on nodes that have `rbd-nbd` and `nbd` kernel modules to map rbd images

NOTE: If `clusterID` parameter is used, then an accompanying Ceph cluster
configuration secret or config files needs to be provided to the running pods.
Refer to [Cluster ID based configuration](../examples/README.md#cluster-id-based-configuration)
for more information. A suggested way to populate the clusterID is to use the
output of `ceph fsid` of the Ceph cluster to be used for provisioning.

**Required secrets:**

Admin credentials are required for provisioning new RBD images `ADMIN_NAME`:
`ADMIN_PASSWORD` - note that the key of the key-value pair is the name of the
client with admin privileges, and the value is its password

If clusterID is specified, then a secret with various keys and values as
specified in `examples/rbd/template-ceph-cluster-ID-secret.yaml` needs to be
created, with the secret name matching the string value provided as the
`clusterID`.

## Deployment with Kubernetes

Requires Kubernetes 1.11

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
pod/csi-rbdplugin-fptqr           2/2       Running   0          21s
pod/csi-rbdplugin-provisioner-0   4/4       Running   0          22s

NAME                                TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)     AGE
service/csi-rbdplugin-provisioner   ClusterIP   10.104.2.130   <none>        12345/TCP   23s
...
```

You can try deploying a demo pod from `examples/rbd` to test the deployment further.

## Deployment with Helm

The same requirements from the Kubernetes section apply here, i.e. Kubernetes
version, privileged flag and shared mounts.

The Helm chart is located in `deploy/rbd/helm`.

**Deploy Helm Chart:**

```bash
helm install ./deploy/rbd/helm
```

The Helm chart deploys all of the required resources to use the CSI RBD driver.
After deploying the chart you can verify the deployment using the instructions
above for verifying the deployment with Kubernetes
