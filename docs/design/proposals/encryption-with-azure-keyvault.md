# Encrypted volumes with Azure Key Vault

Azure Key Vault is a cloud service for securely storing and accessing secrets.
A secret is anything that you want to tightly control access to, such as API
keys, passwords, certificates, or cryptographic keys.

## Connection to Azure Key Vault

Below values are used to establish the connection to the Key Vault
service from the CSI driver and to make use of the secrets
`GetSecret`/`SetSecret`/`DeleteSecret` operations:

```text
* AZURE_VAULT_URL
The URL used to access the Azure Key Vault service.

* AZURE_CLIENT_ID
The Client ID of the Azure application object (also known as the service principal).
This ID serves as the username.

* AZURE_TENANT_ID
The Tenant ID associated with the service principal.

* CLIENT_CERT
The client certificate (which includes the private key and is not password protected)
used for authentication with Azure Key Vault.
```

### Values provided in the connection secret

Considering `AZURE_CLIENT_CERTIFICATE` is sensitive information,
it will be provided as a Kubernetes secret to the Ceph-CSI driver. The Ceph-CSI
KMS plugin interface for the Azure key vault will read the secret name from the
kms configMap and fetch the certificate.

### Values provided in the config map

`AZURE_VAULT_URL`, `AZURE_CLIENT_ID`, `AZURE_TENANT_ID` are part of the
KMS ConfigMap.

### Storage class values or configuration

The Storage class has to be enabled for encryption and `encryptionKMSID` has
to be provided which is the matching value in the kms config map.

## Volume Encrypt or Decrypt Operation

Ceph-CSI generate's unique passphrase for each volume to be used to
encrypt/decrypt. The passphrase is securely store in Azure key vault
using the `SetSecret` operation. At time of decrypt the passphrase is
retrieved from the key vault using the `GetSecret`operation.

## Volume Delete Operation

When the corresponding volume is deleted, the stored secret in the Azure Key
Vault will be deleted.

> Note: Ceph-CSI solely deletes the secret without permanent removal (purging).
