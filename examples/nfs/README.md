# Dynamic provisioning with NFS

The easiest way to try out the examples for dynamic provisioning with NFS, is
to use [Rook Ceph with CephNFS][rook_ceph]. Rook can be used to deploy a Ceph
cluster. Ceph is able to maintain a NFS-Ganesha service with a few commands,
making configuring the Ceph cluster a minimal effort.

## Enabling the Ceph NFS-service

Ceph does not enable the NFS-service by default. In order for Rook Ceph to be
able to configure NFS-exports, the NFS-service needs to be configured first.

In the [Rook Toolbox][rook_toolbox], run the following commands:

```console
ceph osd pool create nfs-ganesha
ceph mgr module enable rook
ceph mgr module enable nfs
ceph orch set backend rook
```

## Create a NFS-cluster

In the directory where this `README` is located, there is an example
`rook-nfs.yaml` file. This file can be used to create a Ceph managed
NFS-cluster with the name "my-nfs".

```console
$ kubectl create -f rook-nfs.yaml
cephnfs.ceph.rook.io/my-nfs created
```

The CephNFS resource will create a NFS-Ganesha Pod and Service with label
`app=rook-ceph-nfs`:

```console
$ kubectl get pods -l app=rook-ceph-nfs
NAME                                      READY   STATUS    RESTARTS   AGE
rook-ceph-nfs-my-nfs-a-5d47f66977-sc2rk   2/2     Running   0          61s
$ kubectl get service -l app=rook-ceph-nfs
NAME                     TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
rook-ceph-nfs-my-nfs-a   ClusterIP   172.30.218.195   <none>        2049/TCP   2m58s
```

## Create a StorageClass

The parameters of the StorageClass reflect mostly what CephFS requires to
connect to the Ceph cluster. All required options are commented clearly in the
`storageclass.yaml` file.

In addition to the CephFS parameters, there are:

- `nfsCluster`: name of the Ceph managed NFS-cluster (here `my-nfs`)
- `server`: hostname/IP/service of the NFS-server (here `172.30.218.195`)

Edit `storageclass.yaml`, and create the resource:

```console
$ kubectl create -f storageclass.yaml
storageclass.storage.k8s.io/csi-nfs-sc created
```

## TODO: next steps

- deploy the NFS-provisioner
- deploy the kubernetes-csi/csi-driver-nfs
- create the CSIDriver object

[rook_ceph]: https://rook.io/docs/rook/latest/ceph-nfs-crd.html
[rook_toolbox]: https://rook.io/docs/rook/latest/ceph-toolbox.html
