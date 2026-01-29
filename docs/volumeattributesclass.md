# VolumeAttributesClass for Volume Modification

Kubernetes offers a method for modifying Volume parameters after they have been
created. This is done through VolumeAttributesClasses and is described in a
[blog
post](https://kubernetes.io/blog/2025/09/08/kubernetes-v1-34-volume-attributes-class).

## Prerequisites

- Kubernetes 1.34 is the first release where support for
  VolumeAttributesClasses (the `ControllerModifyVolume` CSI procedure) is GA.
  Older versions of Kubernetes may not work reliably.
- The Kubernetes CSI external-provisioner (`csi-provisioner` sidecar) release
  needs to be higher or equal to v6.1 (support for secrets).
- The Kubernetes CSI external-resizer (`csi-resizer` sidecar) release
  needs to be higher or equal to v2.1 (support for secrets).

## Secret references in the StorageClass

When setting a VolumeAttributesClass on a PersistentVolumeClaim, the
`ControllerModifyVolume` CSI procedure is called on the provisioner. This
procedure needs secrets (that contain the Ceph credentials) in order to
communicate with the Ceph cluster. Below are the keys that should be set in the
StorageClass' `parameters`:

- `csi.storage.k8s.io/controller-modify-secret-name`
- `csi.storage.k8s.io/controller-modify-secret-namespace`

In addition to the execution of `ControllerModifyVolume`, the Node-plugin needs
access to the Volume on the Ceph cluster to fetch the updated parameters.
Usually NFS and NVMe-oF do not need any Ceph credentials for the Node-plugin,
but for using VolumeAttributesClasses to modify Volumes this is a requirement.

Depending on the storage backend, Volumes are optionally _Staged_ before
getting _Published_. NVMe-oF uses the staging process, and needs credentials
there, hence these parameters should be set:

- `csi.storage.k8s.io/node-stage-secret-name`
- `csi.storage.k8s.io/node-stage-secret-namespace`

NFS does not use the staging process, and only needs the credentials during the
publishing process:

- `csi.storage.k8s.io/node-publish-secret-name`
- `csi.storage.k8s.io/node-publish-secret-namespace`

## Adding support to existing Volumes

PersistentVolumeClaims that have not been created with the right secret
references in the StorageClass will not be modifiable with a
VolumeAttributesClass without manual intervention.

In order to be able to modify parameters with a VolumeAttributesClass,
annotations should be added to the PersistentVolume that is _Bound_ to the
PersistentVolumeClaim. These annotations should refer to the namespace and
secret where the credentials are available (just like the namespace and secret
that would be referenced in the StorageClass).

- `volume.kubernetes.io/controller-modify-secret-name`
- `volume.kubernetes.io/controller-modify-secret-namespace`

Note that these annotations only make it possible for the provisioner to modify
parameters. This does not allow the Node-plugin accessing the Ceph cluster to
fetch updated parameters when the Volume is _staged_ or _published_. The
secrets for staging and publishing can not (easily) be updated after the fact,
these are part of the fixed parameters in the PersistentVolume.
