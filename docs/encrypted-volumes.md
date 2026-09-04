# Encrypted Volumes

> Enabling encryption on volumes created without encryption is **not supported**
>
> Enabling encryption for storage class that has PVs created without encryption
> is **not supported**

Volumes provisioned with Ceph RBD do not have encryption by default. It is
possible to encrypt them with ceph-csi by using LUKS encryption.

## Life-cycle for encrypted volumes

**Create volume**:

* create volume request received
* volume requested to be created in Ceph
* new passphrase is generated and stored in selected KMS if KMS is in use
* encrypted state "encryptionPrepared" is saved in image-meta in Ceph

**Attach volume**:

* attach volume request received
* volume is attached to provisioner container
* on first time attachment
  (no file system on the attached device, checked with blkid)
   * passphrase is retrieved from selected KMS if KMS is in use
   * device is encrypted with LUKS using a passphrase from K8s Secret or KMS
   * image-meta updated to "encrypted" in Ceph
* passphrase is retrieved from selected KMS if KMS is in use
* device is open and device path is changed to use a mapper device
* mapper device is used instead of original one with usual workflow

**Detach volume**:

* mapper device closed and device path changed to original volume path
* volume is detached as usual
* passphrase removed from KMS if needed (with failures ignored)

## Encryption configuration

To encrypt rbd volumes with LUKS you need to set encryption passphrase in
secrets under `encryptionPassphrase` key and switch `encrypted` option in
StorageClass to `"true"`. This is not supported for storage classes that already
have PVs provisioned. The `node-stage-secret-name` and the `provisioner-secret-name`
should carry this key and value for encryption to work.

To use different passphrase you need to have different storage classes and point
to a different K8s secrets `csi.storage.k8s.io/node-stage-secret-name`
and `csi.storage.k8s.io/provisioner-secret-name` which carry new passphrase value
for `encryptionPassphrase` key in these secrets.

## Encryption `metadata` configuration

CephCSI can generate unique passphrase (DEK Data-Encryption-Key) for each volume
to be used to encrypt/decrypt data. The passphrase (DEK) is encrypted by
`encryptionPassphrase` (KEK Key-Encryption-Key) and stored in the image metadata
of the volume.

To encrypt rbd volumes with `metadata` encryption, users need to set
`encrypted: "true"` and `encryptionKMSID` to a unique identifier in storageclass.
This unique identifier should be similar to the
[examples](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/csi-kms-connection-details.yaml).
The configuration must include `"encryptionKMSType": "metadata"`. The
`encryptionPassphrase` is fetched based on the following conditions:

* if `"secretName"` key is specified, `encryptionPassphrase` is fetched from this
  secret and `"secretNamespace"` value is used for namespace if specified else
  Tenant/Kubernetes namespace (i.e., namespace where the PVC was created) is used.
* if `"secretName"` key is not specified, `encryptionPassphrase` is fetched from
  storageclass secrets `csi.storage.k8s.io/provisioner-secret-namespace` /
  `csi.storage.k8s.io/provisioner-secret-name` and
  `csi.storage.k8s.io/node-stage-secret-namespace` /
  `csi.storage.k8s.io/node-stage-secret-name`
  similar to the previous [Encryption Configuration](#encryption-configuration).

## Encryption KMS configuration

To further improve security robustness it is possible to use unique passphrases
generated for each volume and stored in a Key Management System (KMS). Currently
HashiCorp Vault is the only KMS supported.

There are two options to use Hashicorp Vault as a KMS:

1. with Kubernetes ServiceAccount
1. with a Vault Token per Tenant (a Kubernetes Namespace)

To use Vault as KMS set `encryptionKMSID` to a unique identifier for Vault
configuration. You will also need to create vault configuration similar to the
[example](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml)
and use same
`encryptionKMSID`.

To use the Kubernetes ServiceAccount to access Vault, the configuration must
include `encryptionKMSType: "vault"`. If Tenants are expected to place their
Vault Token in a Kubernetes Secret in their Namespace, set `encryptionKMSType:
"vaulttokens"`.

In order for ceph-csi to be able to access the configuration you will need to
have it mounted to csi-rbdplugin containers in both daemonset (so kms client
can be instantiated to encrypt/decrypt volumes) and deployment pods (so kms
client can be instantiated to delete passphrase on volume delete)
`ceph-csi-encryption-kms-config` configmap.

> Note: kms configuration must be a map of string values only
> (`map[string]string`) so for numerical and boolean values make sure to put
> quotes around.

When the Tenants need to provide their own Vault Token, they will need to place
it in a Kubernetes Secret (by default) called `ceph-csi-kms-token`, where the
Vault Token is stored in the `token` key as shown in [the
example](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/tenant-token.yaml).

### Configuring HashiCorp Vault with a single Kubernetes ServiceAccount

Using Vault as KMS you need to configure Kubernetes authentication method as
described in [official
documentation](https://www.vaultproject.io/docs/auth/kubernetes.html).

If token reviewer is used, you will need to configure service account for
that also like in
[example](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/csi-vaulttokenreview-rbac.yaml)
to be able to
review jwt tokens.

Configure a role(s) for service accounts used for ceph-csi:

* provisioner service account (`rbd-csi-provisioner`) requires only **delete**
  permissions to delete passphrases on PVC delete
* nodeplugin service account (`rbd-csi-nodeplugin`) requires **create** and
  **read** permissions to save new keys and retrieve existing

### Configuring Hashicorp Vault with a ServiceAccount per Tenant

For deployments where a single ServiceAccount for accessing Hashicorp Vault is
not suitable, it is possible to configure a ServiceAccount per Tenant to access
the KMS. In order to configure this, each Tenant will need to have its own
ServiceAccount in the Kubernetes Namespace where the volumes are created. The
ServiceAccount is expected to be called `ceph-csi-vault-sa` by default. This
can be changed by setting the `tenantSAName` option to a different value. An
example of the global configuration that can be done in the Kubernetes
Namespace where Ceph-CSI is deployed can be found in
[`kms-config.yaml`](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml)
where the
`encryptionKMSType` is set to `vaulttenantsa`.

Most notably, the Vault Tokens KMS configuration can be used, without the Token
configuration, but with added `tenantSAName` and `vaultRole` options.

Tenants do have the ability to reconfigure parts of the connection details to
the Vault service. It will often be required to set the backend path to a
location where the Tenant can manage the secrets. These changes can be done by
placing a ConfigMap called `ceph-csi-kms-config` in the Tenants Namespace, an
[example](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/tenant-sa.yaml)
is available.

As each ServiceAccount needs to be added to the Vault configuration, the
administrator of the service will need to apply the permissions by creating a
Vault Policy that allows a ServiceAccount to access a key-value store in the
KMS. In the Ceph-CSI automated testing, there is [a Kubernetes Job that sets
this up](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/tenant-token.yaml)
for a single Tenant that uses
the Kubernetes Namespace `tenant`.

### Configuring Amazon KMS

Amazon KMS can be used to encrypt and decrypt the passphrases that are used for
encrypted RBD images. When a volume is created, a passphrase will be generated,
which will be encrypted by the KMS and stored in the volumes metadata. Upon
attaching the volume to a Pod, the worker node requests the KMS to decrypt the
passphrase, after which it can be used to open the device with `cryptsetup` and
provide access to it for the Pod.

There are a few settings that need to be included in the [KMS configuration
file](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml):

1. `KMS_PROVIDER`: should be set to `aws-metadata`.
1. `KMS_SECRET_NAME`: name of the Kubernetes Secret (in the Namespace where
   Ceph-CSI is deployed) which contains the credentials for communicating with
   AWS. This defaults to `ceph-csi-aws-credentials`.
1. `AWS_REGION`: the region where the AWS KMS service is available.

The
[Secret with credentials](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/aws-credentials.yaml)
for
the AWS KMS is expected to contain:

1. `AWS_ACCESS_KEY_ID`: ID of the key to use for encrypting/decrypting
1. `AWS_SECRET_ACCESS_KEY`: secret for the key to use
1. `AWS_SESSION_TOKEN`: *(optional)* session token, usually empty
1. `AWS_CMK_ARN`: Custom Master Key, ARN for the key used to encrypt the
   passphrase

This Secret is expected to be created by the administrator who deployed
Ceph-CSI.

### Configuring Amazon KMS with Amazon STS

Ceph-CSI can be configured to use
[Amazon STS](https://docs.aws.amazon.com/STS/latest/APIReference/welcome.html),
when kubernetes cluster is configured with OIDC identity provider to fetch
credentials to access Amazon KMS. Other functionalities is the same as
[Amazon KMS encryption](#configuring-amazon-kms).

There are a few settings that need to be included in the [KMS configuration
file](../examples/kms/vault/kms-config.yaml):

1. `encryptionKMSType`: should be set to `aws-sts-metadata`.
1. `secretName`: name of the Kubernetes Secret (in the Namespace where
   PVC is created) which contains the credentials for communicating with
   AWS. This defaults to `ceph-csi-aws-credentials`.

The [Secret with
credentials](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/aws-sts-credentials.yaml)
for the AWS KMS
is expected to contain:

1. `awsRoleARN`: Role which will be used access credentials from AWS STS
    and access AWS KMS for encryption.
1. `awsCMKARN`: Custom Master Key, ARN for the key used to encrypt the
   passphrase
1. `awsRegion`: the region where the AWS STS and KMS service is available.

This Secret is expected to be created by the tenant/user in each namespace where
Ceph-CSI is used to create encrypted rbd volumes.

### Configuring Azure key vault

Ceph-CSI can be configured to use
[Azure key vault](https://azure.microsoft.com/en-in/products/key-vault),
for encrypting RBD volumes.

There are a few settings that need to be included in the [KMS configuration
file](../examples/kms/vault/kms-config.yaml):

1. `KMS_PROVIDER`: should be set to `azure-kv`.
1. `AZURE_CERT_SECRET_NAME`: name of the Kubernetes Secret (in the Namespace where
   Ceph-CSI is deployed) which contains the credentials for communicating with
   Azure. This defaults to `ceph-csi-azure-credentials`.
1. `AZURE_VAULT_URL`: URL to access the Azure Key Vault service.
1. `AZURE_CLIENT_ID`: Client ID of the Azure application object (service principal)
   created in Azure Active Directory that serves as the username.
1. `AZURE_TENANT_ID`: Tenant ID of the service principal.

The
[Secret with credentials](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/azure-credentials.yaml)
for
the Azure KMS is expected to contain:

1. `CLIENT_CERT`: The client certificate used for authentication
   with Azure Key Vault.

This Secret is expected to be created by the user in the namespace where Ceph-CSI
is deployed.

### Configuring KMIP KMS

The Key Management Interoperability Protocol (KMIP) is an extensible
communication protocol that defines message formats for the manipulation
of cryptographic keys on a key management server.
Ceph-CSI can be configured to connect to various KMS servers using
[KMIP](https://en.wikipedia.org/wiki/Key_Management_Interoperability_Protocol)
for encrypting RBD volumes.

There are a few settings that need to be included in the [KMS configuration
file](../examples/kms/vault/kms-config.yaml):

1. `KMS_PROVIDER`: should be set to `kmip`.
1. `KMIP_ENDPOINT` KMIP endpoint address.
1. `USE_CRYPTO_RPC`(optional): Indicates whether to use the KMIP Encrypt and
    Decrypt RPC operations. Defaults to `true`. Set to `false` if the KMS does not
    support these RPCs, in which case encryption is performed locally using a
    cipher derived from the key referenced by `UNIQUE_IDENTIFIER` in the secret.
1. `KMIP_SECRET_NAME`(optional): name of the Kubernetes Secret which contains
   the credentials for communicating with KMIP server, defaults to
   `ceph-csi-kmip-credentials`.
1. `TLS_SERVER_NAME`(optional): The endpoint server name. Useful when the
   KMIP endpoint does not have a DNS entry.
1. `READ_TIMEOUT`(optional): Network read timeout, in seconds. The default
   value is 10.
1. `WRITE_TIMEOUT`(optional): Network write timeout, in seconds. The default
   value is 10.

The
[Secret with credentials](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kmip-credentials.yaml)
for
the KMIP KMS is expected to contain:

1. `CA_CERT`: CA certificate that will be used to connect to KMIP server.
1. `CLIENT_CERT`: Client certificate that will be used to connect to KMIP server.
1. `CLIENT_KEY`: Client key that will be used to connect to KMIP server.
1. `UNIQUE_IDENTIFIER`: Unique ID of the key to use for encrypting/decrypting.

## Encryption prerequisites

In order for encryption to work you need to make sure that `dm-crypt` kernel
module is enabled on the nodes running ceph-csi attachers.

If custom image is built for the rbd-plugin instance, make sure that it contains
`cryptsetup` tool installed to be able to use encryption.

## CephFS volume encryption

CephFS volumes use [fscrypt](https://www.kernel.org/doc/html/latest/filesystems/fscrypt.html),
the Linux kernel filesystem-level encryption feature, rather than LUKS block
device encryption. The `fscrypt` userspace tool is used for key and policy
management.

> Enabling encryption on CephFS volumes created without encryption is
> **not supported**

### Enabling CephFS encryption

Set `encrypted: "true"` and `encryptionKMSID` in the CephFS StorageClass,
the same parameters used for RBD:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-cephfs-sc-encrypted
provisioner: cephfs.csi.ceph.com
parameters:
  clusterID: <cluster-id>
  fsName: cephfs
  encrypted: "true"
  encryptionKMSID: "user-ns-secrets-metadata"
  csi.storage.k8s.io/provisioner-secret-name: csi-cephfs-secret
  csi.storage.k8s.io/provisioner-secret-namespace: default
  csi.storage.k8s.io/node-stage-secret-name: csi-cephfs-secret
  csi.storage.k8s.io/node-stage-secret-namespace: default
reclaimPolicy: Delete
```

### KMS compatibility

The KMS configuration is shared with RBD encryption and the same
`ceph-csi-encryption-kms-config` ConfigMap is used. However, not all KMS
backends are supported for fscrypt. Only KMS backends that expose the raw
passphrase or secret directly to the driver work:

* Kubernetes Secrets (`encryptionKMSType: "secrets"`)
* `metadata` type (`encryptionKMSType: "metadata"`)
* HashiCorp Vault (`encryptionKMSType: "vault"` / `"vaulttokens"` /
  `"vaulttenantsa"`)

KMS backends that only wrap/unwrap keys without exposing a raw secret (e.g.
AWS KMS, Azure Key Vault) are **not** compatible with fscrypt.

### Volume layout

Due to how `fscrypt` stores its metadata, the subvolume root is not mounted
directly into the Pod. Instead:

* `/.fscrypt/` — managed by `fscrypt`, contains protector and policy metadata
* `/ceph-csi-encrypted/` — the fscrypt-enabled directory exposed to the Pod

### CephFS encryption prerequisites

* Linux kernel >= v5.4 with `CONFIG_FS_ENCRYPTION=y`
* CephFS kernel client with fscrypt support
  ([Ceph tracker #46690](https://tracker.ceph.com/issues/46690))

See the [CephFS fscrypt design proposal](design/proposals/cephfs-fscrypt.md)
for the full key-management architecture and implementation details.

## Design proposals

The following design proposals document the motivation and implementation
details behind each encryption feature:

| Proposal | Description |
| -------- | ----------- |
| [Encrypted Persistent Volume Claims](design/proposals/encrypted-pvc.md) | Original proposal for LUKS-based RBD volume encryption, including the `EncryptionKMS` / `DEKStore` interfaces and StorageClass parameters |
| [CephFS fscrypt Support](design/proposals/cephfs-fscrypt.md) | Filesystem-level encryption for CephFS volumes using the fscrypt kernel feature and the `fscrypt` userspace tool |
| [Encryption Key Rotation](design/proposals/rbd-pv-key-rotation.md) | Rotation of LUKS Key-Encryption-Keys (KEKs) for encrypted RBD volumes via the CSI-Addons `EncryptionKeyRotation` service |
| [Multi-tenancy with Vault Tokens](design/proposals/encryption-with-vault-tokens.md) | Per-tenant Vault Token support, enabling each tenant to supply their own token from a Kubernetes Secret |
| [Vault ServiceAccount per Tenant](design/proposals/encryption-with-vault-sa.md) | Per-tenant Vault access using a Kubernetes ServiceAccount instead of a token |
| [Azure Key Vault](design/proposals/encryption-with-azure-keyvault.md) | Storing volume passphrases in Azure Key Vault using certificate-based authentication |
| [IBM HPCS / Key Protect](design/proposals/encryption-with-keyprotect.md) | KMS integration with IBM Cloud Hyper Protect Crypto Services and Key Protect |
| [IBM GKLM (KMIP)](design/proposals/encryption-with-gklm.md) | KMS integration with IBM Guardium Key Lifecycle Manager via the KMIP protocol |
