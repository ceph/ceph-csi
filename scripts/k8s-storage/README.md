# Kubernetes external storage e2e test suite

The files in this directory are used by the k8s-e2e-external-storage CI job.
This job runs the [Kubernetes end-to-end external storage tests][1] with
different driver configurations/manifests (in the `driver-*.yaml` files). Each
driver configuration refers to a StorageClass that is used while testing.

The StorageClasses are created with the `create-storageclass.sh` script and the
`sc-*.yaml.in` templates.

The Ceph-CSI Configuration from the `ceph-csi-config` ConfigMap is created with
`create-configmap.sh` after the deployment is finished. The ConfigMap is
referenced in the StorageClasses and contains the connection details for the
Ceph cluster.

[1]: https://github.com/kubernetes/kubernetes/tree/master/test/e2e/storage/external
