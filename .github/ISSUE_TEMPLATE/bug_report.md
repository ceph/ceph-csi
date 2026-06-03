---
name: Bug report
about: Create a report to help us improve

---

# Describe the bug #

A clear and concise description of what the bug is.

# Environment details #

- Image/version of Ceph CSI driver :
- Helm chart version :
- Kernel version :
- Mounter used for mounting PVC (for cephFS its `fuse` or `kernel`. for rbd its
  `krbd` or `rbd-nbd`) :
- Kubernetes cluster version :
- Ceph cluster version :

# Steps to reproduce #

Steps to reproduce the behavior:

1. Setup details: '...'
1. Deployment to trigger the issue '....'
1. See error

# Actual results #

Describe what happened

# Expected behavior #

A clear and concise description of what you expected to happen.

# Logs #

If the issue is in PVC creation, deletion, cloning please attach complete logs
of below containers.

- csi-provisioner and csi-rbdplugin/csi-cephfsplugin container logs from the
  provisioner pod.

If the issue is in PVC resize please attach complete logs of below containers.

- csi-resizer and csi-rbdplugin/csi-cephfsplugin container logs from the
  provisioner pod.

If the issue is in snapshot creation and deletion please attach complete logs
of below containers.

- csi-snapshotter and csi-rbdplugin/csi-cephfsplugin container logs from the
  provisioner pod.

If the issue is in PVC mounting please attach complete logs of below containers.

- csi-rbdplugin/csi-cephfsplugin and driver-registrar container logs from
  plugin pod from the node where the mount is failing.

- if required attach dmesg logs.

**Note:-** If its a rbd issue please provide only rbd related logs, if its a
cephFS issue please provide cephFS logs.

# Additional context #

Add any other context about the problem here.

For example:

Any existing bug report which describe about the similar issue/behavior
