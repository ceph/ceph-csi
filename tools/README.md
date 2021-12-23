# Assorted Tools for developing, testing, maintaining and building Ceph-CSI

## `csi-addons`

`csi-addons` can be used inside the Ceph-CSI container images for testing the
CSI-Addons operations. This makes it possible to develop new operations, even
when there is no CSI-Addons Controller/Sidecar with support for the operation
yet.

## `yamlgen`

`yamlgen` reads deployment configurations from the `api/` package and generates
YAML files that can be used for deploying without advanced automation like
Rook. The generated files are located under `deploy/`.

