# CSI to Flexvolume adapter

## Usage:

### Start Flexvolume adapter for simple nfs flexvolume driver
```
$ sudo ./_output/flexadapter --endpoint tcp://127.0.0.1:10000 --drivername simplenfs --driverpath ./pkg/flexadapter/examples/simplenfs-flexdriver/driver/nfs --nodeid CSINode -v=5
```

### Test using csc
Get ```csc``` tool from https://github.com/rexray/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"simplenfs"	"0.1.0"
```

#### NodePublish a volume
```
$ csc node publish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs --attrib server=a.b.c.d --attrib share=nfs_share nfstestvol
nfstestvol
```

#### NodeUnpublish a volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs nfstestvol
nfstestvol
```

#### Get NodeID
```
$ csc node get-id --endpoint tcp://127.0.0.1:10000
CSINode
```

