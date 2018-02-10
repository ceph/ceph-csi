# Ceph CSI

## Overview

RBD CSI plugin implements an interface between  CSI enabled Container
Orchestrator and CEPH cluster. It allows dynamically provision CEPH
volumes and attach it to workloads.
Current implementation of CSI RBD plugin was tested in Kubernetes environment,
but its code does not rely on any Kubernetes specific calls (WIP to make it k8s agnostic)
and should be able to run with any CSI enabled CO (Containers Orchestration).

An RBD CSI plugin is available to help simplify storage management.
Once user creates PVC with the reference to a RBD storage class, rbd image and 
corresponding PV object gets dynamically created and becomes ready to be used by
workloads.

[Container Storage Interface (CSI)](https://github.com/container-storage-interface/) driver, provisioner, and attacher for Ceph RBD and CephFS

## RBD Plugin
### Configuration Requirements

* Secret object with the authentication key for ceph cluster
* StorageClass with rbdplugin (default CSI RBD plugin name) as a provisioner name
  and information about ceph cluster (monitors, pool, etc) 
* Service Accounts with required RBAC permissions   

### Feature Status

### CSI Spec version
v0.1.0 branch of ceph-csi repo correpsonds to version 0.1.0 of CSI spec, in order for a  plugin
to operate sucessfully, other components: external-attacher, external-provisioner and driver-registrar must
be built for the same CSI spec version.
 
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
$ make container
```
By running:
```
$ docker images | grep rbdplugin
```
You should see the following line in the output:
```
csi_images/rbdplugin   v0.1.0  248ddba297fa        30 seconds ago    431 MB
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
$ kubectl create -f ./deploy/kubernetes/rbd-secrets.yaml 
```
**Important:** rbd-secrets.yaml, must be customized to match your ceph environment.

#### Step 2: Create StorageClass
```
$ kubectl create -f ./deploy/kubernetes/rbd-storage-class.yaml
```
**Important:** rbd-storage-class.yaml, must be customized to match your ceph environment.

#### Step 3: Start CSI CEPH RBD plugin
```
$ kubectl create -f ./deploy/kubernetes/rbdplugin.yaml
```

#### Step 4: Start CSI External Attacher
```
$ kubectl create -f ./deploy/kubernetes/csi-attacher.yaml
```

#### Step 5: Start CSI External Provisioner  
```
$ kubectl create -f ./deploy/kubernetes/csi-provisioner.yaml
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
default       csi-nodeplugin-rbdplugin-qxqtl                      2/2       Running   0          1d           
default       csi-provisioner-0                                   1/1       Running   0          1d           
```

#### Step 7: Create PVC 
```
$ kubectl create -f ./deploy/kubernetes/pvc.yaml
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
# kubectl create -f ./deploy/pod.yaml
```

## CephFS plugin

TODO 

## Troubleshooting

Please submit an issue at:[Issues](https://github.com/ceph/ceph-csi/issues)
