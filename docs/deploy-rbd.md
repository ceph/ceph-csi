# CSI RBD Plugin

The RBD CSI plugin is able to provision new RBD images and attach and mount those to worlkoads.

## Building

CSI RBD plugin can be compiled in a form of a binary file or in a form of a Docker image. When compiled as a binary file, the result is stored in `_output/` directory with the name `rbdplugin`. When compiled as an image, it's stored in the local Docker image store.

Building binary:
```bash
$ make rbdplugin
```

Building Docker image:
```bash
$ make image-rbdplugin
```

## Configuration

**Available command line arguments:**

Option | Default value | Description
------ | ------------- | -----------
`--endpoint` | `unix://tmp/csi.sock` | CSI endpoint, must be a UNIX socket
`--drivername` | `csi-cephfsplugin` | name of the driver (Kubernetes: `provisioner` field in StorageClass must correspond to this value)
`--nodeid` | _empty_ | This node's ID

**Available volume parameters:**

Parameter | Required | Description
--------- | -------- | -----------
`monitors` | yes | Comma separated list of Ceph monitors (e.g. `192.168.100.1:6789,192.168.100.2:6789,192.168.100.3:6789`)
`pool` | yes | Ceph pool into which the RBD image shall be created
`imageFormat` | no | RBD image format. Defaults to `2`. See [man pages](http://docs.ceph.com/docs/mimic/man/8/rbd/#cmdoption-rbd-image-format)
`imageFeatures` | no | RBD image features. Available for `imageFormat=2`. CSI RBD currently supports only `layering` feature. See [man pages](http://docs.ceph.com/docs/mimic/man/8/rbd/#cmdoption-rbd-image-feature)
`csiProvisionerSecretName`, `csiNodePublishSecretName` | for Kubernetes | name of the Kubernetes Secret object containing Ceph client credentials. Both parameters should have the same value
`csiProvisionerSecretNamespace`, `csiNodePublishSecretNamespace` | for Kubernetes | namespaces of the above Secret objects

**Required secrets:**  
Admin credentials are required for provisioning new RBD images
`ADMIN_NAME`: `ADMIN_PASSWORD` - note that the key of the key-value pair is the name of the client with admin privileges, and the value is its password

Also note that CSI RBD expects admin keyring and Ceph config file in `/etc/ceph`.

## Deployment with Kubernetes

Requires Kubernetes 1.11

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged` flag must be set to true for both the API server and the kubelet). Moreover, as stated in the [mount propagation docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation), the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in `deploy/rbd/kubernetes`.

**Deploy RBACs for sidecar containers and node plugins:**

```bash
$ kubectl create -f csi-attacher-rbac.yaml
$ kubectl create -f csi-provisioner-rbac.yaml
$ kubectl create -f csi-nodeplugin-rbac.yaml
```

Those manifests deploy service accounts, cluster roles and cluster role bindings. These are shared for both RBD and CephFS CSI plugins, as they require the same permissions.

**Deploy CSI sidecar containers:**

```bash
$ kubectl create -f csi-rbdplugin-attacher.yaml
$ kubectl create -f csi-rbdplugin-provisioner.yaml
```

Deploys stateful sets for external-attacher and external-provisioner sidecar containers for CSI RBD.

**Deploy RBD CSI driver:**

```bash
$ kubectl create -f csi-rbdplugin.yaml
```

Deploys a daemon set with two containers: CSI driver-registrar and the CSI RBD driver.

## Verifying the deployment in Kubernetes

After successfuly completing the steps above, you should see output similar to this:

```bash
$ kubectl get all
NAME                              READY     STATUS    RESTARTS   AGE
pod/csi-rbdplugin-attacher-0      1/1       Running   0          23s
pod/csi-rbdplugin-fptqr           2/2       Running   0          21s
pod/csi-rbdplugin-provisioner-0   1/1       Running   0          22s

NAME                                TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)     AGE
service/csi-rbdplugin-attacher      ClusterIP   10.109.15.54   <none>        12345/TCP   26s
service/csi-rbdplugin-provisioner   ClusterIP   10.104.2.130   <none>        12345/TCP   23s

...
```

You can try deploying a demo pod from `examples/rbd` to test the deployment further.

