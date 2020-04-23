# ceph-csi-cephfs

The ceph-csi-cephfs chart adds cephfs volume support to your cluster.

## Install Chart

To install the Chart into your Kubernetes cluster

- For helm 2.x

    ```bash
    helm install --namespace "ceph-csi-cephfs" --name "ceph-csi-cephfs" ceph-csi/ceph-csi-cephfs
    ```

- For helm 3.x

    Create the namespace where Helm should install the components with

    ```bash
     create namespace ceph-csi-cephfs
    ```

    Run the installation

    ```bash
    helm install --namespace "ceph-csi-cephfs" "ceph-csi-cephfs" ceph-csi/ceph-csi-cephfs
    ```

After installation succeeds, you can get a status of Chart

```bash
helm status "ceph-csi-cephfs"
```

## Delete Chart

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
