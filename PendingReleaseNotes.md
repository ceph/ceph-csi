# v3.18 Pending Release Notes

## Breaking changes

## Features

## NOTE

## Deprecations

- The `netNamespaceFilePath` configuration option is now deprecated and will be
  removed in a future release. Users should migrate to using host networking for
  CSI plugin pods instead. When this feature is detected, a deprecation warning
  will be logged at the WARNING level.
   - `hostPID` was changed from `true` to `false` in all static manifests
   - Static manifest users with `netNamespaceFilePath` must manually set
     `hostPID: true`
   - OpenShift users must also keep `allowHostPID: true` in their SCC (if they
     were using it)
