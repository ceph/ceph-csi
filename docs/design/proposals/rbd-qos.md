# Design Doc for RBD QoS using cgroup v2

## Introduction

The RBD QoS (Quality of Service) design aims to address the issue of IO noisy
neighbor problems encountered in early Ceph deployments catering to OpenStack
environments. These problems were effectively managed by implementing QEMU
throttling at the virtio-blk/scsi level. To further enhance this,
capacity-based IOPS were introduced, providing a more dynamic experience
similar to public cloud environments.

The challenge arises in virtual environments, where a noisy neighbor can lead
to performance degradation for other instances sharing the same resources.
Although it's uncommon to observe noisy neighbor issues in Kubernetes
environments backed by Ceph storage, the possibility exists. The existing QoS
support with rbd-nbd doesn't apply to krbd, and as rbd-nbd isn't suitable for
container production workloads, a solution is needed for krbd.

To mitigate resource starvation issues, setting QoS at the device level through
cgroup v2 when enabled becomes crucial. This approach guarantees that I/O
capacity isn't overcommitted and is fairly distributed among workloads.

## Dependency

* cgroup v2 must be enabled on the Node
* We might have Kubernetes dependency as well
* Container runtime dependency that supports cgroupv2

## Manual steps for implementing RBD QoS in a Kubernetes Cluster

```bash
[$] ssh root@node1
sh-4.4# chroot /host
sh-5.1# cat /proc/partitions
major minor  #blocks  name

 259        0  125829120 nvme0n1
 259        1       1024 nvme0n1p1
 259        2     130048 nvme0n1p2
 259        3     393216 nvme0n1p3
 259        4  125303791 nvme0n1p4
 259        6   52428800 nvme2n1
   7        0  536870912 loop0
 259        5  536870912 nvme1n1
 252        0   52428800 rbd0
sh-5.1#
```

Once the rbd device is mapped on the node we get the device's major and minor
number we need to set the io limit on the device but we need to find the right
cgroup file where we need to set the limit

Kubernetes/Openshift creates a custom cgroup hierarchy for the pods it created
but start is `/sys/fs/cgroup`  folder

```bash
sh-5.1# cd /sys/fs/cgroup/
sh-5.1# ls
cgroup.controllers cgroup.subtree_control cpuset.mems.effective  io.stat   memory.reclaim   sys-kernel-debug.mount
cgroup.max.depth cgroup.threads  dev-hugepages.mount    kubepods.slice  memory.stat   sys-kernel-tracing.mount
cgroup.max.descendants cpu.pressure  dev-mqueue.mount       machine.slice  misc.capacity   system.slice
cgroup.procs  cpu.stat  init.scope        memory.numa_stat  sys-fs-fuse-connections.mount user.slice
cgroup.stat  cpuset.cpus.effective io.pressure        memory.pressure  sys-kernel-config.mount
```

`kubepods.slice` is the starting point and it contains multiple slices

```bash
sh-5.1# cd kubepods.slice
sh-5.1# ls
cgroup.controllers cpuset.cpus    hugetlb.2MB.rsvd.max       memory.pressure
cgroup.events  cpuset.cpus.effective   io.bfq.weight        memory.reclaim
cgroup.freeze  cpuset.cpus.partition   io.latency        memory.stat
cgroup.kill  cpuset.mems    io.max        memory.swap.current
cgroup.max.depth cpuset.mems.effective   io.pressure        memory.swap.events
cgroup.max.descendants hugetlb.1GB.current   io.stat        memory.swap.high
cgroup.procs  hugetlb.1GB.events   kubepods-besteffort.slice      memory.swap.max
cgroup.stat  hugetlb.1GB.events.local  kubepods-burstable.slice      memory.zswap.current
cgroup.subtree_control hugetlb.1GB.max    kubepods-pod2b38830b_c2d6_4528_8935_b1c08511b1e3.slice  memory.zswap.max
cgroup.threads  hugetlb.1GB.numa_stat   memory.current       misc.current
cgroup.type  hugetlb.1GB.rsvd.current  memory.events        misc.max
cpu.idle  hugetlb.1GB.rsvd.max   memory.events.local       pids.current
cpu.max   hugetlb.2MB.current   memory.high        pids.events
cpu.max.burst  hugetlb.2MB.events   memory.low        pids.max
cpu.pressure  hugetlb.2MB.events.local  memory.max        rdma.current
cpu.stat  hugetlb.2MB.max    memory.min        rdma.max
cpu.weight  hugetlb.2MB.numa_stat   memory.numa_stat
cpu.weight.nice  hugetlb.2MB.rsvd.current  memory.oom.group
```

Based on the QoS of the pod, either our application pod will end up in the
above `kubepods-besteffort.slice` or `kubepods-burstable.slice` or
`kubepods.slice` (Guaranteed QoS) cgroup. The 3 QoS classes are defined
[here](https://kubernetes.io/docs/concepts/workloads/pods/pod-QoS/#quality-of-service-classes)

To identify the right cgroup file, we need pod UUID and container UUID from the
`pod yaml` output

```bash
[$]kubectl get po csi-rbd-demo-pod -oyaml |grep uid
  uid: cdf7b785-4eb7-44f7-99cc-ef53890f4dfd
[$]kubectl get po csi-rbd-demo-pod -oyaml |grep -i containerID
  - containerID: cri-o://77e57fbbc0f0630f41f9f154f4b5fe368b6dcf7bef7dcd75a9c4b56676f10bc9
[$]kubectl get po csi-rbd-demo-pod -oyaml |grep -i qosClass
  qosClass: BestEffort
```

Now check in the `kubepods-besteffort.slice` and identify the right path using
pod UID and container UID

Before that check `io.max` on the application pod and see if there is any limit

```bash
[$]kubectl exec -it csi-rbd-demo-pod -- sh
sh-4.4# cat /sys/fs/cgroup/io.max
sh-4.4#
```

Come back to the Node and find the right cgroup scope

```bash
sh-5.1# cd kubepods-besteffort.slice/kubepods-besteffort-podcdf7b785_4eb7_44f7_99cc_ef53890f4dfd.slice/crio-77e57fbbc0f0630f41f9f154f4b5fe368b6dcf7bef7dcd75a9c4b56676f10bc9.scope/


sh-5.1# echo "252:0 wbps=1048576" > io.max
sh-5.1# cat io.max
252:0 rbps=max wbps=1048576 riops=max wiops=max
```

Now go back to the application pod and check if we have the right limit set

```bash
[$]kubectl exec -it csi-rbd-demo-pod -- sh
sh-4.4# cat /sys/fs/cgroup/io.max
252:0 rbps=max wbps=1048576 riops=max wiops=max
sh-4.4#
```

Note:- We can only support the QoS that cgroup v2 io controller supports, this
means that cumulative read+write QoS limits won't be supported.

Below are the configurations that will be supported

|  Parameter     |  Description     |
|  ---  |  ---  |
|  MaxReadIOPS     | Max read IO operations per second      |
|  MaxWriteIOPS     | Max write IO operations per second      |
|  MaxReadBytesPerSecond     |  Max read bytes per second     |
|  MaxWriteBytesPerSecond     |  Max write bytes per second     |

## Implementation Approach

The solution uses VolumeAttributeClass to configure QoS parameters.

### QoS using VolumeAttributeClass with NodePublish Secret

```yaml
apiVersion: storage.k8s.io/v1alpha1
kind: VolumeAttributesClass
metadata:
  name: silver
parameters:
  MaxReadIOPS: ""
  MaxWriteIOPS: ""
  MaxReadBytesPerSecond: ""
  MaxWriteBytesPerSecond: ""
```

VolumeAttributesClassName is a new parameter in the PVC object the user can
choose from and this can also be updated or removed later.

This new VolumeAttributeClass is designed to keep storage that supports setting
QoS at the storage level which means setting some configuration at the storage
(like QoS for nbd)

#### Implementation Steps

1. Modify CSIDriver object to pass pod details to the NodePublishVolume
   CSI procedure
1. Add support in Ceph-CSI to expose ModifyVolume CSI procedure
1. Ceph-CSI will store QoS in the rbd image metadata
1. During NodePublishVolume operation retrieve the QoS from image metadata
1. Whenever a new pod comes in apply the QoS

#### Container Discovery and QoS Application

When kubelet invokes the NodePublishVolume RPC call, it provides the pod UUID
as part of the request. Ceph-CSI will use this pod UUID to locate the correct
cgroup hierarchy path, following the same approach demonstrated in the manual
steps above.

Since Ceph-CSI cannot determine which specific container within the pod the
RBD volume is being mounted to, the QoS limits (io.max) must be applied to
**all containers** found in the pod's cgroup directory. This ensures that the
QoS limits are enforced regardless of which container is using the volume.

The container discovery process follows these steps:

1. Kubelet provides pod UUID in NodePublishVolume request (via pod info in
   volume context)
1. Ceph-CSI identifies the pod's QoS class (BestEffort, Burstable, or
   Guaranteed) from the cgroup hierarchy
1. Navigate to the appropriate kubepods slice based on QoS class:
   * `kubepods-besteffort.slice` for BestEffort
   * `kubepods-burstable.slice` for Burstable
   * `kubepods.slice` for Guaranteed
1. Locate the pod-specific slice using the pod UUID:
   `kubepods-<qos>-pod<uuid>.slice/`
1. Enumerate all container scopes (e.g., `crio-<container-id>.scope/`) within
   the pod slice
1. Apply io.max limits to each container's cgroup by writing to
   `<container-scope>/io.max`

Example path construction:
`/sys/fs/cgroup/kubepods-besteffort.slice/kubepods-besteffort-podcdf7b785_4eb7_44f7_99cc_ef53890f4dfd.slice/crio-77e57fbbc0f0630f41f9f154f4b5fe368b6dcf7bef7dcd75a9c4b56676f10bc9.scope/io.max`

This approach ensures QoS enforcement across all containers in the pod,
addressing the limitation that the specific target container is not known at
NodePublishVolume time.

#### Secret Management

This solution requires a secret to communicate with the ceph cluster during
NodePublishVolume to retrieve the latest QoS settings from the image metadata.

**StorageClass Configuration:**

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd-sc
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: <cluster-id>
  pool: <pool-name>
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: default
  csi.storage.k8s.io/controller-expand-secret-name: csi-rbd-secret
  csi.storage.k8s.io/controller-expand-secret-namespace: default
  csi.storage.k8s.io/node-stage-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-stage-secret-namespace: default
  csi.storage.k8s.io/node-publish-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-publish-secret-namespace: default
```

**Fallback to CSI ConfigMap:**

If the NodePublishVolumeSecret is not specified in the StorageClass, Ceph-CSI
will fall back to the default secret configured in the CSI ConfigMap. This
provides flexibility for existing deployments without requiring StorageClass
updates.

**CSI ConfigMap Configuration:**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ceph-csi-config
data:
  config.json: |-
    [
      {
        "clusterID": "<cluster-id>",
        "monitors": ["<mon1>", "<mon2>", "<mon3>"],
        "rbd": {
          "nodePublishSecretRef": {
            "name": "csi-rbd-secret",
            "namespace": "default"
          }
        }
      }
    ]
```

The secret lookup order:

1. Check PV for `csi.storage.k8s.io/node-publish-secret-name` (from
   StorageClass)
1. If not found, use the `rbd.nodePublishSecretRef` from CSI ConfigMap for
   the corresponding clusterID
1. If neither is available, fail the NodePublishVolume operation with a
   clear error message (if QOS is specified).

#### Advantages of this Approach

1. **No Restore/Clone operation required** - QoS can be changed on existing
   PVCs via VolumeAttributesClass.
1. **Flexible secret management** - Supports both StorageClass-level and
   cluster-level default secrets.

### References

Some of the useful links that helped me to understand cgroup v2 and how to set
QoS on the device.

* [Kubernetes cgroup v2
  Architecture](https://kubernetes.io/docs/concepts/architecture/cgroups/)
* [cgroup v2 kernel doc](https://docs.kernel.org/admin-guide/cgroup-v2.html)
* [ceph RBD QoS tracker](https://tracker.ceph.com/issues/36191)
* [cgroup v2 io
  controller](https://facebookmicrosites.github.io/cgroup2/docs/io-controller.html)
* [Kubernetes IOPS
  issue](https://github.com/kubernetes/kubernetes/issues/92287)
