# CSI NFS driver


## Kubernetes
### Requirements

The folllowing feature gates and runtime config have to be enabled to deploy the driver

```
FEATURE_GATES=CSIPersistentVolume=true,MountPropagation=true
RUNTIME_CONFIG="storage.k8s.io/v1alpha1=true"
```

Mountprogpation requries support for privileged containers. So, make sure privileged containers are enabled in the cluster.

### Example local-up-cluster.sh

```ALLOW_PRIVILEGED=true FEATURE_GATES=CSIPersistentVolume=true,MountPropagation=true RUNTIME_CONFIG="storage.k8s.io/v1alpha1=true" LOG_LEVEL=5 hack/local-up-cluster.sh```

### Deploy

```kubectl -f deploy/kubernetes create```

### Example Nginx application
Please update the NFS Server & share information in nginx.yaml file.

```kubectl -f examples/kubernetes/nginx.yaml create```

## Using CSC tool

### Start NFS driver
```
$ sudo ../../_output/flexadapter --endpoint tcp://127.0.0.1:10000 --drivername simplenfs --driverpath ./examples/simplenfs-flexdriver/driver/nfs --nodeid CSINode -v=3
```

## Test
Get ```csc``` tool from https://github.com/thecodeteam/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugininfo --endpoint tcp://127.0.0.1:10000
"NFS"	"0.1.0"
```

### Get supported versions
```
$ csc identity supportedversions --endpoint tcp://127.0.0.1:10000
0.1.0
```

#### NodePublish a volume
```
$ export NFS_SERVER="Your Server IP (Ex: 10.10.10.10)"
$ export NFS_SHARE="Your NFS share"
$ csc node publishvolume --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs --attrib server=$NFS_SERVER --attrib share=$NFS_SHARE nfstestvol
nfstestvol
```

#### NodeUnpublish a volume
```
$ csc node unpublishvolume --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs nfstestvol
nfstestvol
```

#### Get NodeID
```
$ csc node getid --endpoint tcp://127.0.0.1:10000
CSINode
```

