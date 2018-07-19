# Ceph CSI

## Overview

Ceph CSI plugins implement an interface between CSI enabled Container
Orchestrator and CEPH cluster. It allows dynamically provision CEPH
volumes and attach it to workloads.
Current implementation of Ceph CSI plugins was tested in Kubernetes environment (requires Kubernetes 1.10+),
but the code does not rely on any Kubernetes specific calls (WIP to make it k8s agnostic)
and should be able to run with any CSI enabled CO (Containers Orchestration).

[Container Storage Interface (CSI)](https://github.com/container-storage-interface/) driver, provisioner, and attacher for Ceph RBD and CephFS

## RBD Plugin

An RBD CSI plugin is available to help simplify storage management.
Once user creates PVC with the reference to a RBD storage class, rbd image and 
corresponding PV object gets dynamically created and becomes ready to be used by
workloads.

### Configuration Requirements

* Secret object with the authentication key for ceph cluster
* StorageClass with rbdplugin (default CSI RBD plugin name) as a provisioner name
  and information about ceph cluster (monitors, pool, etc) 
* Service Accounts with required RBAC permissions   

### Feature Status

### 1.9: Alpha

**Important:** `CSIPersistentVolume` and `MountPropagation`
[feature gates must be enabled starting in 1.9](#enabling-the-alpha-feature-gates).
Also API server must run with running config set to: `storage.k8s.io/v1alpha1` 

### Compiling
CSI RBD plugin can be compiled in a form of a binary file or in a form of a container. When compiled
as a binary file, it gets stored in \_output folder with the name rbdplugin. When compiled as a container,
the resulting image is stored in a local docker's image store. 

To compile just a binary file:
```
$ make rbdplugin
```

To build a container:
```
$ make rbdplugin-container
```
By running:
```
$ docker images | grep rbdplugin
```
You should see the following line in the output:
```
quay.io/cephcsi/rbdplugin              v0.2.0                            76369a8f8528        15 minutes ago      372.5 MB
```

### Testing

#### Prerequisite

##### Enable Mount Propagation in Docker 

Comment out `MountFlags=slave` in docker systemd service then restart docker service.
```bash
# systemctl daemon-reload
# systemctl restart docker
```

##### Enable Kubernetes Feature Gates

Enable features `MountPropagation=true,CSIPersistentVolume=true` and runtime config `storage.k8s.io/v1alpha1=true`

#### Step 1: Create Secret 
```
$ kubectl create -f ./deploy/rbd/kubernetes/rbd-secrets.yaml 
```
**Important:** rbd-secrets.yaml, must be customized to match your ceph environment.

#### Step 2: Create StorageClass
```
$ kubectl create -f ./deploy/rbd/kubernetes/rbd-storage-class.yaml
```
**Important:** rbd-storage-class.yaml, must be customized to match your ceph environment.

#### Step 3: Start CSI CEPH RBD plugin
```
$ kubectl create -f ./deploy/rbd/kubernetes/rbdplugin.yaml
```

#### Step 4: Start CSI External Attacher
```
$ kubectl create -f ./deploy/rbd/kubernetes/csi-attacher.yaml
```

#### Step 5: Start CSI External Provisioner  
```
$ kubectl create -f ./deploy/rbd/kubernetes/csi-provisioner.yaml
```
**Important:** Deployment yaml files includes required Service Account definitions and
required RBAC rules.

#### Step 6: Check status of CSI RBD plugin  
```
$ kubectl get pods | grep csi 
```

The following output should be displayed:

```
NAMESPACE     NAME                                                READY     STATUS    RESTARTS   AGE          
default       csi-attacher-0                                      1/1       Running   0          1d           
default       csi-rbdplugin-qxqtl                                 2/2       Running   0          1d           
default       csi-provisioner-0                                   1/1       Running   0          1d           
```

#### Step 7: Create PVC 
```
$ kubectl create -f ./deploy/rbd/kubernetes/pvc.yaml
```

#### Step 8: Check status of provisioner PV  
```
$ kubectl get pv
```

The following output should be displayed:

```
NAME                                                          CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS    CLAIM             STORAGECLASS   REASON    AGE
kubernetes-dynamic-pvc-1b19ddf1-0047-11e8-85ab-760f2eed12ea   5Gi        RWO            Delete           Bound     default/csi-pvc   rbdv2                    10s
```

```
$ kubectl describe pv kubernetes-dynamic-pvc-1b19ddf1-0047-11e8-85ab-760f2eed12ea
Name:            kubernetes-dynamic-pvc-1b19ddf1-0047-11e8-85ab-760f2eed12ea
Annotations:     csi.volume.kubernetes.io/volume-attributes={"monitors":"192.168.80.233:6789","pool":"kubernetes"}
                 csiProvisionerIdentity=1516716490787-8081-rbdplugin  <------ !!!
                 pv.kubernetes.io/provisioned-by=rbdplugin
StorageClass:    rbdv2              <------ !!!
Status:          Bound              <------ !!!
Claim:           default/csi-pvc    <------ !!!
Reclaim Policy:  Delete
Access Modes:    RWO
VolumeMode:      Filesystem
Capacity:        5Gi
Message:         
Source:
    Type:    CSI <------ !!!
```

#### Step 9: Create a test pod

```bash
# kubectl create -f ./deploy/rbd/pod.yaml
```

## CephFS plugin

A CephFS CSI plugin is available to help simplify storage management.
Once user creates PVC with the reference to a CephFS CSI storage class, corresponding
PV object gets dynamically created and becomes ready to be used by workloads.

### Configuration Requirements

* Secret object with the authentication user ID `userID` and key `userKey` for ceph cluster
* StorageClass with csi-cephfsplugin (default CSI CephFS plugin name) as a provisioner name
  and information about ceph cluster (monitors, pool, rootPath, ...)
* Service Accounts with required RBAC permissions

Mounter options: specifies whether to use FUSE or ceph kernel client for mounting. By default, the plugin will probe for `ceph-fuse`. If this fails, the kernel client will be used instead. Command line argument `--volumemounter=[fuse|kernel]` overrides this behaviour.

StorageClass options:
* `provisionVolume: "bool"`: if set to true, the plugin will provision and mount a new volume. Admin credentials `adminID` and `adminKey` are required in the secret object, since this also creates a dedicated RADOS user used for mounting the volume.
* `rootPath: /path-in-cephfs`: required field if `provisionVolume=true`. CephFS is mounted from the specified path. User credentials `userID` and `userKey` are required in the secret object.
* `mounter: "kernel" or "fuse"`: (optional) per-StorageClass mounter configuration. Overrides the default mounter.

### Feature Status

### 1.9: Alpha

**Important:** `CSIPersistentVolume` and `MountPropagation`
[feature gates must be enabled starting in 1.9](#enabling-the-alpha-feature-gates).
Also API server must run with running config set to: `storage.k8s.io/v1alpha1`

* `kube-apiserver` must be launched with `--feature-gates=CSIPersistentVolume=true,MountPropagation=true`
  and `--runtime-config=storage.k8s.io/v1alpha1=true`
* `kube-controller-manager` must be launched with `--feature-gates=CSIPersistentVolume=true`
* `kubelet` must be launched with `--feature-gates=CSIPersistentVolume=true,MountPropagation=true`

### Compiling
CSI CephFS plugin can be compiled in a form of a binary file or in a form of a container. When compiled
as a binary file, it gets stored in \_output folder with the name cephfsplugin. When compiled as a container,
the resulting image is stored in a local docker's image store. 

To compile just a binary file:
```
$ make cephfsplugin
```

To build a container:
```
$ make cephfsplugin-container
```
By running:
```
$ docker images | grep cephfsplugin
```
You should see the following line in the output:
```
quay.io/cephcsi/cephfsplugin              v0.2.0                            79482e644593        4 minutes ago       305MB
```

### Testing

#### Prerequisite

##### Enable Mount Propagation in Docker 

Comment out `MountFlags=slave` in docker systemd service then restart docker service.
```
# systemctl daemon-reload
# systemctl restart docker
```

##### Enable Kubernetes Feature Gates

Enable features `MountPropagation=true,CSIPersistentVolume=true` and runtime config `storage.k8s.io/v1alpha1=true`

#### Step 1: Create Secret 
```
$ kubectl create -f ./deploy/cephfs/kubernetes/secret.yaml
```
**Important:** secret.yaml, must be customized to match your ceph environment.

#### Step 2: Create StorageClass
```
$ kubectl create -f ./deploy/cephfs/kubernetes/cephfs-storage-class.yaml
```
**Important:** cephfs-storage-class.yaml, must be customized to match your ceph environment.

#### Step 3: Start CSI CEPH CephFS plugin
```
$ kubectl create -f ./deploy/cephfs/kubernetes/cephfsplugin.yaml
```

#### Step 4: Start CSI External Attacher
```
$ kubectl create -f ./deploy/cephfs/kubernetes/csi-attacher.yaml
```

#### Step 5: Start CSI External Provisioner  
```
$ kubectl create -f ./deploy/cephfs/kubernetes/csi-provisioner.yaml
```
**Important:** Deployment yaml files includes required Service Account definitions and
required RBAC rules.

#### Step 6: Check status of CSI CephFS plugin  
```
$ kubectl get pods | grep csi 
csi-attacher-0           1/1       Running   0          6m
csi-cephfsplugin-hmqpk   2/2       Running   0          6m
csi-provisioner-0        1/1       Running   0          6m
```

#### Step 7: Create PVC 
```
$ kubectl create -f ./deploy/cephfs/kubernetes/pvc.yaml
```

#### Step 8: Check status of provisioner PV  
```
$ kubectl get pv
NAME                                     CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS    CLAIM                    STORAGECLASS   REASON    AGE
kubernetes-dynamic-pv-715cef0b30d811e8   5Gi        RWX            Delete           Bound     default/csi-cephfs-pvc   csi-cephfs               5s
```

```
$ kubectl describe pv kubernetes-dynamic-pv-715cef0b30d811e8
Name:            kubernetes-dynamic-pv-715cef0b30d811e8
Labels:          <none>
Annotations:     pv.kubernetes.io/provisioned-by=csi-cephfsplugin
StorageClass:    csi-cephfs
Status:          Bound
Claim:           default/csi-cephfs-pvc
Reclaim Policy:  Delete
Access Modes:    RWX
Capacity:        5Gi
Message:         
Source:
    Type:    CSI (a Container Storage Interface (CSI) volume source)
    Driver:      ReadOnly:  %v

    VolumeHandle:                                                                    csi-cephfsplugin
%!(EXTRA string=csi-cephfs-7182b779-30d8-11e8-bf01-5254007d7491, bool=false)Events:  <none>
```

#### Step 9: Create a test pod

```
$ kubectl create -f ./deploy/cephfs/kubernetes/pod.yaml
```

## Troubleshooting

Please submit an issue at:[Issues](https://github.com/ceph/ceph-csi/issues)
