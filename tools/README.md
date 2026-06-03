# Assorted Tools for maintaining and building Ceph-CSI

## `yamlgen`

`yamlgen` reads deployment configurations from the `api/` package and generates
YAML files that can be used for deploying without advanced automation like
Rook. The generated files are located under `deploy/`.
