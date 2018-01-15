# CSI to Flexvolume adapter

## Usage:

### Start Flexvolume adapter for simple nfs flexvolume driver
```
$ sudo ../_output/flexadapter --endpoint tcp://127.0.0.1:10000 --drivername simplenfs --driverpath ./examples/simple-nfs-flexdriver/nfs --nodeid CSINode
```

### Test using csc
Get ```csc``` tool from https://github.com/chakri-nelluri/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugininfo --endpoint tcp://127.0.0.1:10000
"simplenfs"	"0.1.0"
```

### Get supported versions
```
$ csc identity supportedversions --endpoint tcp://127.0.0.1:10000
0.1.0
```

#### NodePublish a volume
```
$ csc node publishvolume --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs --attrib server=a.b.c.d --attrib share=nfs_share nfstestvol
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

