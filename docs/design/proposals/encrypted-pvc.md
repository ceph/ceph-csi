# Encrypted Persistent Volume Claims

## Proposal
Subject of this proposal is to add an encrypt feature to ceph-csi in order to allow encryption at rbd volumes level.

Some but not all the benefits of this approach:
- guarantee encryption in transit to rbd without using messenger v2
- extra security layer to application with special regulatory needs
- at rest encryption can be disabled to selectively allow encryption only where required

## Document Terminology
- volume encryption: encryption of a volume attached by rbd
- encryption at rest: encryption of physical disk done by ceph
- LUKS: Linux Unified Key Setup: stores all of the needed setup information for dm-crypt on the disk
- dm-crypt: linux kernel device-mapper crypto target
- cryptsetup: the command line tool to interface with dm-crypt

## Proposed Solution

The proposed solution in this document, is to address the volume encryption requirement by using dm-crypt module through cryptsetup cli interface.

### Implementation Summary

- Encryption is implemented using cryptsetup with LUKS extension.
  A good introduction to LUKS and dm-crypt in general can be found [here](https://wiki.archlinux.org/index.php/Dm-crypt/Device_encryption#Encrypting_devices_with_cryptsetup)
  Functions to implement necesary interaction are implemented in a separate dmcrypt.go file.
  1. applyLUKS
  2. openLUKS
  3. getPassphrase
  4. closeLUKS

- `createRBDImage`: refactored to call `applyLUKS`, before returning, if encrypted volume option is set.
- `applyLUKS`: attaches newly generated image to the worker node where provisioner is running and sets up LUKS using `cryptsetup` command.
  The function:
  - resolves the passphrase from a `getPassphrase` function according to kms volume options.
  - A new passphrase, if not found, is stored in the kms using target volume name (i.e. pvc-8a710f4c934811e9) for lookup
  - applies LUKS to the attached rbd volume
  - detaches the volume from the worker node where the provisioner is running.
- `attachRBDImage`: refactored into a strategy function to call `openLUKS` when an encrypted volume attach is requested.
- `openLUKS`: looks up for a passphrase matching the volume name, return the new device path in the form: `/dev/mapper/luks-pvc-8a710f4c934811e9`.
  On the woker node where the attach is scheduled:
  ```
  [user@worker-01 ~]$ lsblk
  NAME                            MAJ:MIN RM  SIZE RO TYPE  MOUNTPOINT
  sda                               8:0    0   10G  0 disk
  └─sda1                            8:1    0   10G  0 part  /
  sdb                               8:16   0   20G  0 disk
  rbd0                            253:0    0    1G  0 disk
  └─luks-pvc-8a710f4c934811e9 252:0    0 1020M  0 crypt /var/lib/kubelet/pods/9eaceaef-936c-11e9-b396-005056af3de0/volumes/kubernetes.io~csi/pvc-8a710f4c934811e9/mount
  ```
- `detachRBDImage`: calls `closeLUKS` function to remove the LUKS mapping before detaching the volume.


- StorageClass extended with following parameters:
  1. `encrypted: bool`
  2. `kms: map`
  cephcsi plugin will support different kms vendors with different type of authentication

#### Annotated YAML for RBD StorageClass
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: csi-rbd
provisioner: rbd.csi.ceph.com
parameters:

    monitors: <monitor-1>, <monitor-2>, <monitor-3>

    # Ceph pool into which the RBD image shall be created
    pool: <pool-name>

    # RBD image format. Defaults to "2".
    imageFormat: "2"

    imageFeatures: layering

    # Encryption instructions
    encrypted: "true"
    kms:
        # The type of kms we want to connect to: Barbican, aws kms or others can be supported
        type: "vault"

        # Auth methods offered by the kms.
        # In this case one of https://www.vaultproject.io/docs/auth/index.html
        auth:
            method: "kubernetes"

    csiProvisionerSecretName: csi-rbd-secret
    csiProvisionerSecretNamespace: default
    csiNodePublishSecretName: csi-rbd-secret
    csiNodePublishSecretNamespace: default

    # Ceph users for operating RBD
    adminid: admin
    userid: username

reclaimPolicy: Delete
```
