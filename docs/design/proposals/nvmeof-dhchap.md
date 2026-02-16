# NVMe-oF DH-CHAP Authentication Design Document

## Overview

This document describes the implementation of DH-CHAP authentication for
the Ceph NVMe-oF CSI driver. DH-CHAP provides secure authentication
between worker nodes and NVMe-oF storage gateways.

---

## What is DH-CHAP

DH-CHAP (Diffie-Hellman Challenge Handshake Authentication Protocol)
 is an authentication protocol defined in the NVMe specification.
 It works like a password system for storage connections - before
  a node can access storage, it must prove its identity using a cryptographic key.

**Two authentication modes:**

- **Unidirectional**: The host (worker node) proves its identity to the
  storage gateway

- **Bidirectional**: Both the host and the storage gateway prove
  their identities to each other (more secure)

**Key format:** DH-CHAP keys follow the NVMe spec format:
`DHHC-1:01:base64_encoded_key:crc32_checksum`

---

## Architecture

### Key Components

- **SecurityKeyNVMEOFManager**: Manages authentication keys using a
   pluggable KMS (Key Management System) backend
- **DEKStore**: Stores encrypted keys (RBD metadata for testing, Vault for production)
- **Controller Server**: Generates keys during volume publish operations
- **Node Server**: Retrieves keys during `nvme connect` command and
   right before volume mount operations

### Key Storage Pattern

Keys are stored using a two-layer encryption model borrowed from volume encryption:

```text
Plain DH-CHAP Key
    ↓ -encrypt with KEK (e.g. from K8s Secret)-
Encrypted DH-CHAP Key
    ↓ -store in DEKStore-
RBD Metadata / Vault
```

Each node-subsystem pair gets a unique keys

---

## Configuration

### StorageClass Parameters

Introduce 2 new params in the storageClass.

- dhchapMode - can be  none/unidirectional/bidirectional or omitted.

- authenticationKMSID - when dhchap mode is enabled, Admin needs to
  provide kmsID.

if `authenticationKMSID` was not provided- the NVMe-oF CSI driver
will use rbd-metadata-kms
and will save the encrypted dh-chap key in rbd metadata.

if `dhchapMode` was not provided (no matter if `authenticationKMSID`
provided or not), it means the NVMe-oF CSI driver will
skip this feature.

In production user must specify authenticationKMSID

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-nvmeof-secure
provisioner: nvmeof.csi.ceph.com
parameters:
  # Existing parameters
  pool: "rbd-pool"
  subsystemNQN: "nqn.2016-06.io.ceph:subsystem.test"

  # New DH-CHAP parameters
  dhchapMode: "unidirectional"           # none/unidirectional/bidirectional
  authenticationKMSID: "metadata"        # if is omitted, the defaults is "metadata"
```

In addition to the storageClass, The admin also must add encryptionKMSType
param in NVMe-oF ConfigMap.

### KMS ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: csi-kms-connection-details
  namespace: rook-ceph
data:
  metadata: |-
    {
      "encryptionKMSType": "metadata"
    }
```
