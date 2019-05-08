# CSI CephFS plugin

The CSI CephFS plugin is able to both provision new CephFS volumes
and attach and mount existing ones to workloads.

## Building

CSI CephFS plugin can be compiled in the form of a binary file or in the form
of a Docker image.
When compiled as a binary file, the result is stored in `_output/`
directory with the name `cephcsi`.
When compiled as an image, it's stored in the local Docker image store
with name `cephfsplugin`.

Building binary:

```bash
make cephcsi
```

Building Docker image:

```bash
make image-cephfsplugin
```

## Configuration

**Available command line arguments:**

Option              | Default value         | Description
--------------------|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
`--endpoint`        | `unix://tmp/csi.sock` | CSI endpoint, must be a UNIX socket
`--drivername`      | `cephfs.csi.ceph.com`    | name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)
`--nodeid`          | _empty_               | This node's ID
`--volumemounter`   | _empty_               | default volume mounter. Available options are `kernel` and `fuse`. This is the mount method used if volume parameters don't specify otherwise. If left unspecified, the driver will first probe for `ceph-fuse` in system's path and will choose Ceph kernel client if probing failed.
`--metadatastorage` | _empty_               | Whether metadata should be kept on node as file or in a k8s configmap (`node` or `k8s_configmap`)
`--mountcachedir` | _empty_               | volume mount cache info save dir. If left unspecified, the dirver will not record mount info, or it will save mount info and when driver restart it will remount volume it cached.

**Available environmental variables:**

`KUBERNETES_CONFIG_PATH`: if you use `k8s_configmap` as metadata store, specify
the path of your k8s config file (if not specified, the plugin will assume
you're running it inside a k8s cluster and find the config itself).

`POD_NAMESPACE`: if you use `k8s_configmap` as metadata store, `POD_NAMESPACE`
is used to define in which namespace you want the configmaps to be stored

**Available volume parameters:**

Parameter                                                                                           | Required                                               | Description
----------------------------------------------------------------------------------------------------|--------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
`monitors`                                                                                          | yes                                                    | Comma separated list of Ceph monitors (e.g. `192.168.100.1:6789,192.168.100.2:6789,192.168.100.3:6789`)
`monValueFromSecret`                                                                                | one of `monitors` and `monValueFromSecret` must be set | a string pointing the key in the credential secret, whose value is the mon. This is used for the case when the monitors' IP or hostnames are changed, the secret can be updated to pick up the new monitors. If both `monitors` and `monValueFromSecret` are set and the monitors set in the secret exists, `monValueFromSecret` takes precedence.
`mounter`                                                                                           | no                                                     | Mount method to be used for this volume. Available options are `kernel` for Ceph kernel client and `fuse` for Ceph FUSE driver. Defaults to "default mounter", see command line arguments.
`provisionVolume`                                                                                   | yes                                                    | Mode of operation. BOOL value. If `true`, a new CephFS volume will be provisioned. If `false`, an existing volume will be used.
`pool`                                                                                              | for `provisionVolume=true`                             | Ceph pool into which the volume shall be created
`rootPath`                                                                                          | for `provisionVolume=false`                            | Root path of an existing CephFS volume
`csi.storage.k8s.io/provisioner-secret-name`, `csi.storage.k8s.io/node-stage-secret-name`           | for Kubernetes                                         | name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value
`csi.storage.k8s.io/provisioner-secret-namespace`, `csi.storage.k8s.io/node-stage-secret-namespace` | for Kubernetes                                         | namespaces of the above Secret objects

**Required secrets for `provisionVolume=true`:**
Admin credentials are required for provisioning new volumes

* `adminID`: ID of an admin client
* `adminKey`: key of the admin client

**Required secrets for `provisionVolume=false`:**
User credentials with access to an existing volume

* `userID`: ID of a user client
* `userKey`: key of a user client

Notes on volume size: when provisioning a new volume, `max_bytes` quota
attribute for this volume will be set to the requested volume size (see [Ceph
quota documentation](http://docs.ceph.com/docs/mimic/cephfs/quota/)). A request
for a zero-sized volume means no quota attribute will be set.

## Deployment with Kubernetes

Requires Kubernetes 1.13

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged`
flag must be set to true for both the API server and the kubelet). Moreover, as
stated in the [mount propagation
docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation),
the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in `deploy/cephfs/kubernetes`.

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
kubectl create -f csi-cephfsplugin-provisioner.yaml
```

Deploys stateful set of provision which includes external-provisioner
,external-attacher for CSI CephFS.

**Deploy CSI CephFS driver:**

```bash
kubectl create -f csi-cephfsplugin.yaml
```

Deploys a daemon set with two containers: CSI node-driver-registrar and
the CSI CephFS driver.

## Verifying the deployment in Kubernetes

After successfully completing the steps above, you should see output similar to this:

```bash
$ kubectl get all
NAME                                 READY     STATUS    RESTARTS   AGE
pod/csi-cephfsplugin-provisioner-0   3/3       Running   0          25s
pod/csi-cephfsplugin-rljcv           2/2       Running   0          24s

NAME                                   TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)     AGE
service/csi-cephfsplugin-provisioner   ClusterIP   10.101.78.75     <none>        12345/TCP   26s
...
```

You can try deploying a demo pod from `examples/cephfs` to test the deployment further.

### Notes on volume deletion

Volumes that were provisioned dynamically (i.e. `provisionVolume=true`) are
allowed to be deleted by the driver as well, if the user chooses to do
so.Otherwise, the driver is forbidden to delete such volumes - attempting to
delete them is a no-op.
