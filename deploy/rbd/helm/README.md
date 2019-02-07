# ceph-csi-rbd

The ceph-csi-rbd chart adds rbd volume support to your cluster.


## Install Chart

To install the Chart into your Kubernetes cluster :

```bash
helm install --namespace "ceph-csi-rbd" --name "ceph-csi-rbd" ceph-csi/ceph-csi-rbd
```

After installation succeeds, you can get a status of Chart

```bash
helm status "ceph-csi-rbd"
```

If you want to delete your Chart, use this command:

```bash
helm delete  --purge "ceph-csi-rbd"
```

