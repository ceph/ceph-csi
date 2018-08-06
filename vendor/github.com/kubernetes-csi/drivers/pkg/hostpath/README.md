# CSI Hostpath driver

## Usage:

### Build hostpathplugin
```
$ make hostpath
```

### Start Hostpath driver
```
$ sudo ./_output/hostpathplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode -v=5
```

### Test using csc
Get ```csc``` tool from https://github.com/rexray/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"csi-hostpath"  "0.1.0"
```

#### Create a volume
```
$ csc controller new --endpoint tcp://127.0.0.1:10000 --cap 1,block CSIVolumeName
CSIVolumeID
```

#### Delete a volume
```
$ csc controller del --endpoint tcp://127.0.0.1:10000 CSIVolumeID
CSIVolumeID
```

#### Validate volume capabilities
```
$ csc controller validate-volume-capabilities --endpoint tcp://127.0.0.1:10000 --cap 1,block CSIVolumeID
CSIVolumeID  true
```

#### NodePublish a volume
```
$ csc node publish --endpoint tcp://127.0.0.1:10000 --cap 1,block --target-path /mnt/hostpath CSIVolumeID
CSIVolumeID
```

#### NodeUnpublish a volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/hostpath CSIVolumeID
CSIVolumeID
```

#### Get NodeID
```
$ csc node get-id --endpoint tcp://127.0.0.1:10000
CSINode
```
