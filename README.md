# Ceph CSI
[Container Storage Interface (CSI)](https://github.com/container-storage-interface/) driver, provisioner, and attacher for Ceph RBD and CephFS

# Prerequisite

## Enable Mount Propagation in Docker 

Comment out `MountFlags=slave` in docker systemd service then restart docker service.
```bash
# systemctl daemon-reload
# systemctl restart docker
```

## Enable Kubernetes Feature Gates

Enable features `MountPropagation=true,CSIPersistentVolume=true` and runtime config `storage.k8s.io/v1alpha1=true`

# Build

```bash
# make container
```

# Test

## Start rbdplugin and driver registrar

```bash
# kubectl create -f deploy/kubernetes/rbdplugin.yaml
```

### Start CSI external volume provisioner

```bash
# kubectl create -f deploy/kubernetes/csi-provisioner.yaml
```

### Start CSI external volume attacher

```
# kubectl create -f deploy/kubernetes/csi-attacher.yaml
```

### Verify all componets are ready

```bash
# kubectl get pod
NAME                             READY     STATUS    RESTARTS   AGE
csi-attacher-0                   1/1       Running   0          6s
csi-nodeplugin-rbdplugin-kwhhc   2/2       Running   0          6m
csi-provisioner-0                1/1       Running   0          1m
```

### Create a CSI storage class


### Create a PVC

### Create a Pod

