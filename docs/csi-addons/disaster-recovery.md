# Failover and Failback In Disaster Recovery

[RBD mirroring](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/)
 is an asynchronous replication of RBD images between multiple Ceph clusters.
 This capability is available in two modes:

* Journal-based: Every write to the RBD image is first recorded
 to the associated journal before modifying the actual image.
 The remote cluster will read from this associated journal and
 replay the updates to its local image.
* Snapshot-based: This mode uses periodically scheduled or
 manually created RBD image mirror-snapshots to replicate
 crash-consistent RBD images between clusters.

This documentation assumes that `rbd mirroring` is set up between
 two clusters.
For more information on how to set up rbd mirroring, refer to
 [ceph documentation](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/).

## Deploy the Volume Replication CRD

Volume Replication Operator is a kubernetes operator that provides common
 and reusable APIs for storage disaster recovery.
 It is based on [csi-addons/spec](https://github.com/csi-addons/spec)
 specification and can be used by any storage provider.

Volume Replication Operator follows controller pattern and provides
 extended APIs for storage disaster recovery.
 The extended APIs are provided via Custom Resource Definition (CRD).

>:bulb: For more information, please refer to the
> [volume-replication-operator](https://github.com/csi-addons/volume-replication-operator).

* Deploy the `VolumeReplicationClass` CRD

```bash
    kubectl create -f https://raw.githubusercontent.com/csi-addons/volume-replication-operator/release-v0.1/config/crd/bases/replication.storage.openshift.io_volumereplicationclasses.yaml

 customresourcedefinition.apiextensions.k8s.io/volumereplicationclasses.replication.storage.openshift.io created

```

* Deploy the `VolumeReplication` CRD

```bash
   kubectl create -f https://raw.githubusercontent.com/csi-addons/volume-replication-operator/release-v0.1/config/crd/bases/replication.storage.openshift.io_volumereplications.yaml

 customresourcedefinition.apiextensions.k8s.io/volumereplications.replication.storage.openshift.io created created
 ```

The VolumeReplicationClass and VolumeReplication CRDs are now created.

>:bulb: **Note:** Use the latest available release for Volume Replication Operator.
> See [releases](https://github.com/csi-addons/volume-replication-operator/branches)
> for more information.

### Add RBAC rules for Volume Replication Operator

Add the below mentioned rules to `rbd-external-provisioner-runner`
 ClusterRole in [csi-provisioner-rbac.yaml](https://github.com/ceph/ceph-csi/blob/release-v3.3/deploy/rbd/kubernetes/csi-provisioner-rbac.yaml)

```yaml
  - apiGroups: ["replication.storage.openshift.io"]
    resources: ["volumereplications", "volumereplicationclasses"]
    verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
  - apiGroups: ["replication.storage.openshift.io"]
    resources: ["volumereplications/finalizers"]
    verbs: ["update"]
  - apiGroups: ["replication.storage.openshift.io"]
    resources: ["volumereplications/status"]
    verbs: ["get", "patch", "update"]
  - apiGroups: ["replication.storage.openshift.io"]
    resources: ["volumereplicationclasses/status"]
    verbs: ["get"]
```

### Deploy the Volume Replication Sidecar

To deploy `volume-replication` sidecar container in `csi-rbdplugin-provisioner`
 pod, add the following yaml to
 [csi-rbdplugin-provisioner deployment](https://github.com/ceph/ceph-csi/blob/release-v3.3/deploy/rbd/kubernetes/csi-rbdplugin-provisioner.yaml).

```yaml
        - name: volume-replication
          image: quay.io/csiaddons/volumereplication-operator:v0.1.0
          args :
            - "--metrics-bind-address=0"
            - "--leader-election-namespace=$(NAMESPACE)"
            - "--driver-name=rbd.csi.ceph.com"
            - "--csi-address=$(ADDRESS)"
            - "--rpc-timeout=150s"
            - "--health-probe-bind-address=:9998"
            - "--leader-elect=true"
          env:
            - name: ADDRESS
              value: unix:///csi/csi-provisioner.sock
            - name: NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          imagePullPolicy: "IfNotPresent"
          volumeMounts:
            - name: socket-dir
              mountPath: /csi
```

## VolumeReplicationClass and VolumeReplication

### VolumeReplicationClass

*VolumeReplicationClass* is a cluster scoped resource that contains
 driver related configuration parameters. It holds the storage admin
 information required for the volume replication operator.

### VolumeReplication

*VolumeReplication* is a namespaced resource that contains references
 to storage object to be replicated and VolumeReplicationClass
 corresponding to the driver providing replication.

>:bulb: For more information, please refer to the
> [volume-replication-operator](https://github.com/csi-addons/volume-replication-operator).

Let's say we have a *PVC* (rbd-pvc) in BOUND state; created using
 *StorageClass* with `Retain` reclaimPolicy.

```bash
kubectl get pvc --context=cluster-1

 NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
 rbd-pvc   Bound    pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec   1Gi        RWO            csi-rbd-sc   44s
 ```

* Create Volume Replication Class on cluster-1

    ```yaml
    $cat <<EOF | kubectl --context=cluster1 apply -f -
    apiVersion: replication.storage.openshift.io/v1alpha1
    kind: VolumeReplicationClass
    metadata:
      name: rbd-volumereplicationclass
    spec:
      provisioner: rbd.csi.ceph.com
      parameters:
        mirroringMode: snapshot
        schedulingInterval: "12m"
        schedulingStartTime: "16:18:43"
        replication.storage.openshift.io/replication-secret-name: csi-rbd-secret
        replication.storage.openshift.io/replication-secret-namespace: default
    EOF
    ```

>:bulb: **Note:** The `schedulingInterval` can be specified in formats of
> minutes, hours or days using suffix `m`,`h` and `d` respectively.
> The optional schedulingStartTime can be specified using the ISO 8601
> time format.

* Once VolumeReplicationClass is created,create a Volume Replication for
 the PVC which we intend to replicate to secondary cluster.

    ```yaml
    $cat <<EOF | kubectl --context=cluster-1 apply -f -
    apiVersion: replication.storage.openshift.io/v1alpha1
    kind: VolumeReplication
    metadata:
      name: pvc-volumereplication
    spec:
      volumeReplicationClass: rbd-volumereplicationclass
      replicationState: primary
      dataSource:
        apiGroup: ""
        kind: PersistentVolumeClaim
        name: rbd-pvc # Name of the PVC to which mirroring to be enabled.
    EOF
    ```

>:memo: *VolumeReplication* is a namespace scoped object. Thus,
> it should be created in the same namespace as of PVC.

`replicationState` is the state of the volume being referenced.
 Possible values are primary, secondary, and resync.

* `primary` denotes that the volume is primary.
* `secondary` denotes that the volume is secondary.
* `resync` denotes that the volume needs to be resynced.

To check VolumeReplication CR status:

```yaml
kubectl get volumereplication pvc-volumereplication  --context=cluster-1 -oyaml

...
spec:
 dataSource:
   apiGroup: ""
   kind: PersistentVolumeClaim
   name: rbd-pvc
 replicationState: primary
 volumeReplicationClass: rbd-volumereplicationclass
status:
 conditions:
 - lastTransitionTime: "2021-05-04T07:39:00Z"
   message: ""
   observedGeneration: 1
   reason: Promoted
   status: "True"
   type: Completed
 - lastTransitionTime: "2021-05-04T07:39:00Z"
   message: ""
   observedGeneration: 1
   reason: Healthy
   status: "False"
   type: Degraded
 - lastTransitionTime: "2021-05-04T07:39:00Z"
   message: ""
   observedGeneration: 1
   reason: NotResyncing
   status: "False"
   type: Resyncing
 lastCompletionTime: "2021-05-04T07:39:00Z"
 lastStartTime: "2021-05-04T07:38:59Z"
 message: volume is marked primary
 observedGeneration: 1
 state: Primary
```

* Take a backup of PVC and PV object on primary cluster(cluster-1)

   * Take backup of the PVC `rbd-pvc`

    ```bash
    kubectl get pvc rbd-pvc -oyaml >pvc-backup.yaml
    ```

   * Take a backup of the PV, corresponding to the PVC

    ```bash
    kubectl get pv/pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec -oyaml >pv_backup.yaml
    ```

>:bulb: We can also take backup using external tools like **Velero**.
> Refer [velero documentation]((https://velero.io/docs/main/)) for more information.

* Restoring on the secondary cluster(cluster-2)

   * Create storageclass on the secondary cluster

  ```bash
    kubectl create -f examples/rbd/storageclass.yaml --context=cluster-2

   storageclass.storage.k8s.io/csi-rbd-sc created
  ```

   * Create VolumeReplicationClass on the secondary cluster

  ```bash
        cat <<EOF | kubectl --context=cluster-2 apply -f -
        apiVersion: replication.storage.openshift.io/v1alpha1
        kind: VolumeReplicationClass
        metadata:
          name: rbd-volumereplicationclass
        spec:
          provisioner: rbd.csi.ceph.com
          parameters:
            mirroringMode: snapshot
            replication.storage.openshift.io/replication-secret-name: csi-rbd-secret
            replication.storage.openshift.io/replication-secret-namespace: default
        EOF

  volumereplicationclass.replication.storage.openshift.io/rbd-volumereplicationclass created
  ```

   * If Persistent Volumes and Claims are created manually
   on the secondary cluster, remove the `claimRef` on the
   backed up PV objects in yaml files; so that the PV can
   get bound to the new claim on the secondary cluster.

  ```yaml
    ...
    spec:
        accessModes:
        - ReadWriteOnce
        capacity:
          storage: 1Gi
        claimRef:
          apiVersion: v1
          kind: PersistentVolumeClaim
          name: rbd-pvc
          namespace: default
          resourceVersion: "64252"
          uid: 65dc0aac-5e15-4474-90f4-7a3532c621ec
        csi:
    ...
  ```

* Apply the Persistent Volume backup from the primary cluster

```bash
    kubectl create -f pv-backup.yaml --context=cluster-2

 persistentvolume/pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec created
```

* Apply the Persistent Volume claim from the restored backup

```bash
    kubectl create -f pvc-backup.yaml --context=cluster-2

 persistentvolumeclaim/rbd-pvc created
```

```bash
  kubectl get pvc --context=cluster-2

 NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
 rbd-pvc   Bound    pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec   1Gi        RWO            csi-rbd-sc   44s
```

## Planned Migration

> Use cases: Datacenter maintenance, Technology refresh, Disaster avoidance, etc.

### Failover

The failover operation is the process of switching production to a
 backup facility (normally your recovery site). In the case of Failover,
 access to the image on the primary site should be stopped.
The image should now be made *primary* on the secondary cluster so that
 the access can be resumed there.

:memo: As mentioned in the pre-requisites, periodic or one time backup of
 the application should be available for restore on the secondary site (cluster-b).

Follow the below steps for planned migration of workload from primary
 cluster to secondary cluster:

* Scale down all the application pods which are using the
 mirrored PVC on the Primary Cluster
* Take a back up of PVC and PV object from the primary cluster.
 This can be done using some backup tools like
 [velero](https://velero.io/docs/main/).
* Update `replicationState` to `secondary` in VolumeReplication CR at Primary Site.
 When the operator sees this change, it will pass the information down to the
  driver via GRPC request to mark the dataSource as `secondary`.
* If you are manually recreating the PVC and PV on the secondary cluster,
 remove the `claimRef` section in the PV objects.
* Recreate the storageclass, PVC, and PV objects on the secondary site.
* As you are creating the static binding between PVC and PV, a new PV won’t
 be created here, the PVC will get bind to the existing PV.
* Create the VolumeReplicationClass on the secondary site.
* Create the VolumeReplications for all the PVC’s for which mirroring
 is enabled
   * `replicationState` should be `primary` for all the PVC’s on
   the secondary site.
* Check whether the image is marked `primary` on the secondary site
 by verifying it in VolumeReplication CR status.
* Once the Image is marked as `primary`, the PVC is now ready
 to be used. Now, we can scale up the applications to use the PVC.

>:memo: **WARNING**: In Async Disaster recovery use case, we don't
> get the complete data. We will only get the crash-consistent data
> based on the snapshot interval time.

### Failback

To perform a failback operation to primary cluster in case of planned migration
, just repeat the Failback steps in vice-versa.

>:memo: **Remember**: We can skip the backup-restore operations
> in case of failback if the required yamls are already present on
> the primary cluster. Any new PVCs will still need to be restored on the
> primary site.

## Disaster Recovery

> Use cases: Natural disasters, Power failures, System failures, and crashes, etc.

### Failover (abrupt shutdown)

In case of Disaster recovery, create VolumeReplication CR at the Secondary Site.
 Since the connection to the Primary Site is lost, the operator automatically
  sends a GRPC request down to the driver to forcefully mark the dataSource as `primary`.

* If you are manually creating the PVC and PV on the secondary cluster, remove
 the claimRef section in the PV objects.
* Create the storageclass, PVC, and PV objects on the secondary site.
* As you are creating the static binding between PVC and PV, a new PV won’t be
 created here, the PVC will get bind to the existing PV.
* Create the VolumeReplicationClass and VolumeReplication CR on the secondary site.
* Check whether the image is `primary` on secondary site, by verifying in
 the VolumeReplication CR status.
* Once the Image is marked as `primary`, the PVC is now ready to be used. Now,
 we can scale up the applications to use the PVC.

### Failback (post-disaster recovery)

Once the failed cluster is recovered on the primary site and you want to failback
 from secondary site, follow the below steps:

* Update the VolumeReplication CR replicationState
 from `primary` to `secondary` on the primary site.
* Scale down the applications on the secondary site.
* Update the VolumeReplication CR replicationState from `primary` to
 `secondary` in secondary site.
* On the primary site, verify that the VolumeReplication status is marked as
 volume ready to use
* Once the volume is marked to ready to use, change the replicationState state
 from `secondary` to `primary` in primary site.
* Scale up the applications again on the primary site.
