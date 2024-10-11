# v3.13 Pending Release Notes

## Breaking Changes

## Features

- util: a log message "Slow GRPC" is now emitted when
  CSI GRPC call outlives its deadline [PR](https://github.com/ceph/ceph-csi/pull/4847)
- CSI metrics for sidecars are now exposed at `POD_IP`:`SIDECAR_ENDPOINT`/`metrics`
  path. Check sidecar container spec for `SIDECAR_ENDPOINT`
  value [PR](https://github.com/ceph/ceph-csi/pull/4887)
- helm: Support setting nodepluigin and provisioner annotations

## NOTE
