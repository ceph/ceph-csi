# Encrypted Persistent Volume Claims

## Proposal

Subject of this proposal is to add support for encryption of RBD volumes in
Ceph-CSI with type LUKS version 2.

Some but not all the benefits of this approach:

* guarantee encryption in transit to rbd without using messenger v2
* extra security layer to application with special regulatory needs
* at rest encryption can be disabled to selectively allow encryption only where
  required

## Document Terminology

* volume encryption: encryption of a volume attached by rbd
* encryption at rest: encryption of physical disk done by ceph
* LUKS: Linux Unified Key Setup: stores all the needed setup information for
  dm-crypt on the disk
* dm-crypt: linux kernel device-mapper crypto target
* cryptsetup: the command line tool to interface with dm-crypt

## Proposed Solution

The proposed solution in this document, is to address the volume encryption
requirement by using dm-crypt module through cryptsetup cli interface.

### Implementation Summary

* Encryption is implemented using cryptsetup with LUKS extension. A good
  introduction to LUKS and dm-crypt in general can be found
  [here](https://wiki.archlinux.org/index.php/Dm-crypt/Device_encryption#Encrypting_devices_with_cryptsetup)
  Functions to implement necessary interaction are implemented in a separate
  `cryptsetup.go` file.
   * LuksFormat
   * LuksOpen
   * LuksClose
   * LuksStatus

* `CreateVolume`: refactored to prepare for encryption (tag image that it
  requires encryption later), before returning, if encrypted volume option is
  set.
* `NodeStageVolume`: refactored to call `encryptDevice` method on the very first
  volume attach request
* `NodeStageVolume`: refactored to open encrypted device (`openEncryptedDevice`)
* `openEncryptedDevice`: looks up for a passphrase matching the volume id,
  returns the new device path in the form: `/dev/mapper/luks-<volume_id>`. On
  the worker node where the attach is scheduled:

  ```shell
  $ lsblk
  NAME                            MAJ:MIN RM  SIZE RO TYPE  MOUNTPOINT
  sda                               8:0    0   10G  0 disk
  └─sda1                            8:1    0   10G  0 part  /
  sdb                               8:16   0   20G  0 disk
  rbd0                            253:0    0    1G  0 disk
  └─luks-pvc-8a710f4c934811e9 252:0    0 1020M  0 crypt /var/lib/kubelet/pods/9eaceaef-936c-11e9-b396-005056af3de0/volumes/kubernetes.io~csi/pvc-8a710f4c934811e9/mount
  ```

* `detachRBDDevice`: calls `LuksClose` function to remove the LUKS mapping
  before detaching the volume.

* StorageClass extended with following parameters:
 1. `encrypted` ("true" or "false")
 2. `encryptionKMSID` (string representing kms configuration of choice)
    ceph-csi plugin may support different kms vendors with different type of
    authentication

* New KMS Configuration created.

#### Annotated YAML for RBD StorageClass

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd
provisioner: rbd.csi.ceph.com
parameters:
  # String representing Ceph cluster configuration
  clusterID: <cluster-id>
  # ceph pool
  pool: rbd

  # RBD image features, CSI creates image with image-format 2
  # CSI RBD currently supports only `layering` feature.
  imageFeatures: layering

  # The secrets have to contain Ceph credentials with required access
  # to the 'pool'.
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: default
  csi.storage.k8s.io/controller-expand-secret-name: csi-rbd-secret
  csi.storage.k8s.io/controller-expand-secret-namespace: default
  csi.storage.k8s.io/node-stage-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-stage-secret-namespace: default
  # Specify the filesystem type of the volume. If not specified,
  # csi-provisioner will set default as `ext4`.
  csi.storage.k8s.io/fstype: ext4

  # Encrypt volumes
  encrypted: "true"

  # Use external key management system for encryption passphrases by specifying
  # a unique ID matching KMS ConfigMap. The ID is only used for correlation to
  # configmap entry.
  encryptionKMSID: <kms-id>

reclaimPolicy: Delete
```

And kms configuration:

```yaml
---
apiVersion: v1
kind: ConfigMap
data:
  config.json: |-
    {
      "<kms-id>": {
        "encryptionKMSType": "kmsType",
        kms specific config...
      }
    }
metadata:
  name: ceph-csi-encryption-kms-config
```

### Implementation Details

The main components that are used to support encrypted volumes:

1. the `EncryptionKMS` interface

* an instance is configured per volume object (`rbdVolume.KMS`)
* used to authenticate with a master key or token
* can store the KEK (Key-Encryption-Key) for encrypting and decrypting the
  DEKs (Data-Encryption-Key)

1. the `DEKStore` interface

* saves and fetches the DEK (Data-Encryption-Key)
* can be provided by a KMS, or by other components (like `rbdVolume`)

1. the `VolumeEncryption` type

* combines `EncryptionKMS` and `DEKStore` into a single place
* easy to configure from other components or subsystems
* provides a simple API for all KMS operations
