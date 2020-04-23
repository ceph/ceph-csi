# ceph-csi-rbd

The ceph-csi-rbd chart adds rbd volume support to your cluster.

## Install Chart

To install the Chart into your Kubernetes cluster

- For helm 2.x

    ```bash
    helm install --namespace "ceph-csi-rbd" --name "ceph-csi-rbd" ceph-csi/ceph-csi-rbd
    ```

- For helm 3.x

    Create the namespace where Helm should install the components with

    ```bash
    kubectl create namespace "ceph-csi-rbd"
    ```

    Run the installation

    ```bash
    helm install --namespace "ceph-csi-rbd" "ceph-csi-rbd" ceph-csi/ceph-csi-rbd
    ```

After installation succeeds, you can get a status of Chart

```bash
helm status "ceph-csi-rbd"
```

## Delete Chart

If you want to delete your Chart, use this command

- For helm 2.x

    ```bash
    helm delete --purge "ceph-csi-rbd"
    ```

- For helm 3.x

    ```bash
    helm uninstall "ceph-csi-rbd" --namespace "ceph-csi-rbd"
    ```

If you want to delete the namespace, use this command

```bash
kubectl delete namespace ceph-csi-rbd
```
