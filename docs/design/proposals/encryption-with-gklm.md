# Encrypted volumes with IBM GKLM

IBM Guardium Key Lifecycle Manager (GKLM) is a centralized key management system
that helps manage encryption keys and certificates across the enterprise. GKLM
supports the Key Management Interoperability Protocol (KMIP), an industry-standard
protocol for communication with key management systems. To support this KMS
integration in Ceph-CSI and enable GKLM users to perform volume encryption
operations, the following details are provided.

## Connection to IBM GKLM service

GKLM uses the KMIP protocol for communication. The following parameters/values
can be used to establish the connection to the GKLM service from the CSI driver
and to perform encryption operations:

```text
* KMS_PROVIDER
Must be set to "kmip" to use the KMIP protocol.

* USE_CRYPTO_RPC
Indicates whether cryptographic operations should be handled by the KMS server.
The KMS provider must support the Encrypt and Decrypt RPC methods defined
in the KMIP specification.
For GKLM, this setting MUST be `false`.

* KMIP_ENDPOINT
The GKLM server endpoint address (e.g., "gklm-server:5696").

* KMIP_SECRET_NAME
Name of the Kubernetes Secret containing the credentials for
communicating with the GKLM server. Defaults to "ceph-csi-kmip-credentials".

* KMS_SERVICE_NAME (optional)
A unique name for the key management service within the project.

* TLS_SERVER_NAME (optional)
The endpoint server name. Useful when the GKLM endpoint does not have
a DNS entry. This SAN on the returned certificate must match this.

* READ_TIMEOUT (optional)
Network read timeout, in seconds. The default value is 10.

* WRITE_TIMEOUT (optional)
Network write timeout, in seconds. The default value is 10.
```

### Values provided in the connection Secret

The Secret contains TLS certificates for secure communication with the GKLM
server and the unique identifier of the encryption key:

```text
* CA_CERT
The GKLM server's certificate that is marked as key serving certificate.

* CLIENT_CERT
Client certificate that will be used to connect to the GKLM server.

* CLIENT_KEY
Client key that will be used to connect to the GKLM server.

* UNIQUE_IDENTIFIER
UUID of the symmetric key pair in GKLM to use for encryption/decryption.
```

The Ceph-CSI KMS plugin interface for GKLM will read the Secret name from the
KMS ConfigMap and fetch these values.

### Values provided in the ConfigMap

The KMS configuration is stored in a ConfigMap (typically named
`ceph-csi-encryption-kms-config`) with the following structure:

```json
{
  "gklm-test": {
    "KMS_PROVIDER": "kmip",
    "KMS_SERVICE_NAME": "gklm",
    "USE_CRYPTO_RPC": "false",
    "KMIP_ENDPOINT": "sklmapp.sklm.svc.cluster.local:5696",
    "KMIP_SECRET_NAME": "ceph-csi-kmip-credentials",
    "TLS_SERVER_NAME": "sklmapp.sklm.svc.cluster.local",
    "READ_TIMEOUT": 10,
    "WRITE_TIMEOUT": 10
  }
}
```

### Storage class configuration

As with other KMS integrations, the StorageClass must be configured for
encryption with the `encryptionKMSID` parameter matching the key in the KMS ConfigMap:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd-sc-gklm
parameters:
  encrypted: "true"
  encryptionKMSID: "gklm-test" # This matches the JSON key for GKLM config above
  # ... other parameters
```

## Prerequisites

Before using GKLM for volume encryption, you must:

1. Create a symmetric key pair (256-bit) in the GKLM server
1. Obtain the UUID of this key
1. Provide this UUID as the `UNIQUE_IDENTIFIER` in the Kubernetes Secret
1. Ensure `USE_CRYPTO_RPC` is set to `"false"` in the KMS ConfigMap

## Volume Encrypt and Decrypt Operations

GKLM integration differs from traditional KMIP implementations in how it handles
encryption and decryption:

This approach uses GKLM's `Get` RPC operations instead of the
`Encrypt/Decrypt` operations used by other KMIP servers. The actual
encryption/decryption of the DEK happens locally using the key
fetched from GKLM by its UUID.

**Note:** The above will only apply if `USE_CRYPTO_RPC` is set to false in
the KMS ConfigMap.

### Encryption Process

1. The CSI driver connects to the GKLM server using KMIP protocol and TLS
   certificates
1. It fetches the encryption key from GKLM using the `Get` operation with the
   provided `UNIQUE_IDENTIFIER`
1. A Data Encryption Key (DEK) is generated locally for LUKS encryption
1. The DEK is encrypted locally using AES-256-GCM with the fetched encryption key
1. The encrypted DEK and nonce are stored in the RBD image metadata (similar to
   other KMS integrations)

### Decryption Process

1. The encrypted DEK and nonce are retrieved from the RBD image metadata
1. The encryption key is fetched from GKLM using the `Get` operation
1. The DEK is decrypted locally using AES-256-GCM with the fetched key and nonce
1. The decrypted DEK is used for LUKS volume decryption

## Integration Protocol

GKLM integration uses the [KMIP protocol](https://en.wikipedia.org/wiki/Key_Management_Interoperability_Protocol)
for communication. The [KMIP Go library](https://github.com/gemalto/kmip-go)
provides the client SDK to interact with the KMIP server and perform key
management operations.

## Additional References

- [IBM Guardium Key Lifecycle Manager Documentation](https://www.ibm.com/docs/en/gklm)
- [KMIP Specification](https://docs.oasis-open.org/kmip/spec/v1.4/kmip-spec-v1.4.html)
