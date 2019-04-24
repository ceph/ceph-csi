# ceph-csi-cephfs

The ceph-csi-cephfs chart adds cephfs volume support to your cluster.

## Install Chart

To install the Chart into your Kubernetes cluster

```bash
helm install --namespace "ceph-csi-cephfs" --name "ceph-csi-cephfs" ceph-csi/ceph-csi-cephfs
```

After installation succeeds, you can get a status of Chart

```bash
helm status "ceph-csi-cephfs"
```

If you want to delete your Chart, use this command

```bash
helm delete --purge "ceph-csi-cephfs"
```

If you want to delete the namespace, use this command

```bash
kubectl delete namespace ceph-csi-rbd
```
