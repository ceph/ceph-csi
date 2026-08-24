# Kubernetes external storage e2e test suite

The files in this directory are used by the k8s-e2e-external-storage CI job.
This job runs the [Kubernetes end-to-end external storage tests][1] with
different driver configurations/manifests (in the `driver-*.yaml` files). Each
driver configuration refers to a StorageClass that is used while testing.

The StorageClasses are created with the `create-storageclasses.sh` script and the
`sc-*.yaml.in` templates.

The VolumeSnapshotClasses are created with the
`create-volumesnapshotclasses.sh` script and the
`volumesnapshotclass-*.yaml.in` templates.

The Ceph-CSI Configuration from the `ceph-csi-config` ConfigMap is created with
`create-configmap.sh` after the deployment is finished. The ConfigMap is
referenced in the StorageClasses and contains the connection details for the
Ceph cluster.

## Driver-specific Configuration

### NVMeoF Driver

The NVMeoF driver requires additional configuration parameters that can be set
via environment variables before running `create-storageclasses.sh`. All
parameters are auto-detected from the `rook-ceph-nvmeof` service and pod if not
set:

- `GATEWAY_ADDRESS`: NVMeoF gateway IP address (auto-detected from the
  `rook-ceph-nvmeof` service `clusterIP`)
- `SHORT_HOSTNAME`: short hostname of the gateway (auto-detected from the
  `rook-ceph-nvmeof` service name)
- `POD_ADDRESS`: IP address of the NVMeoF gateway pod (auto-detected from the
  `rook-ceph-nvmeof` pod); needed because the gateway is deployed in the
  `rook-ceph` namespace and its short hostname cannot be resolved from the
  ceph-csi testing namespace
- `LISTENERS`: JSON array of listener configurations (auto-generated from
  `POD_ADDRESS`, port `4420`, and `SHORT_HOSTNAME` if not set)

Example:

```bash
export GATEWAY_ADDRESS=10.242.64.32
export POD_ADDRESS=10.242.64.32
export SHORT_HOSTNAME=rook-ceph-nvmeof
./create-storageclasses.sh
```

[1]: https://github.com/kubernetes/kubernetes/tree/master/test/e2e/storage/external
