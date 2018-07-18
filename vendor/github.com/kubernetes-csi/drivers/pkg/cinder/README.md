# CSI Cinder driver

## Kubernetes

### Requirements

The following feature gates and runtime config have to be enabled to deploy the driver.

```
FEATURE_GATES=CSIPersistentVolume=true,MountPropagation=true
RUNTIME_CONFIG="storage.k8s.io/v1alpha1=true"
```

Mountprogpation requires support for privileged containers. So, make sure privileged containers are enabled in the cluster.

### Example local-up-cluster.sh

```ALLOW_PRIVILEGED=true FEATURE_GATES=CSIPersistentVolume=true,MountPropagation=true RUNTIME_CONFIG="storage.k8s.io/v1alpha1=true" LOG_LEVEL=5 hack/local-up-cluster.sh```

### Deploy

Encode your ```cloud.conf``` file content using base64.

```base64 -w 0 cloud.conf```

Update ```cloud.conf``` configuration in ```deploy/kubernetes/csi-secret-cinderplugin.yaml``` file
by using the result of the above command.

```kubectl -f deploy/kubernetes create```

### Example Nginx application

```kubectl -f examples/kubernetes/nginx.yaml create```

## Using CSC tool

### Start Cinder driver
```
$ sudo ./_output/cinderplugin --endpoint tcp://127.0.0.1:10000 --cloud-config /etc/cloud.conf --nodeid CSINodeID
```

### Test using csc
Get ```csc``` tool from https://github.com/rexray/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"csi-cinderplugin"	"0.1.0"
```

#### Create a volume
```
$ csc controller new --endpoint tcp://127.0.0.1:10000 CSIVolumeName
CSIVolumeID
```

#### Delete a volume
```
$ csc controller del --endpoint tcp://127.0.0.1:10000 CSIVolumeID
CSIVolumeID
```

#### ControllerPublish a volume
```
$ csc controller publish --endpoint tcp://127.0.0.1:10000 --node-id=CSINodeID CSIVolumeID
CSIVolumeID	"DevicePath"="/dev/xxx"
```

#### ControllerUnpublish a volume
```
$ csc controller unpublish --endpoint tcp://127.0.0.1:10000 --node-id=CSINodeID CSIVolumeID
CSIVolumeID
```

#### NodePublish a volume
```
$ csc node publish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/cinder --pub-info DevicePath="/dev/xxx" CSIVolumeID
CSIVolumeID
```

#### NodeUnpublish a volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/cinder CSIVolumeID
CSIVolumeID
```

#### Get NodeID
```
$ csc node get-id --endpoint tcp://127.0.0.1:10000
CSINodeID
```
