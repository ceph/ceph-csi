# Fetching Ceph user credentials from KMS

## Proposal

Ceph-CSI supports several KMS implementations for storing and
fetching DEKs (Data Encryption Keys) to support encryption of RBD volumes.
Focus of this proposal is to extend the existing KMS implementation to fetch
Ceph User credentials used for creating, deleting, mapping and resizing RBD volumes.

### Benefits

- For Ceph-CSI setup where Ceph is deployed externally, either as
  a standalone or through Rook, one needs to manually obtain the Ceph user keyring(s)
  and create the K8s required secrets. With this feature,
  sensitive keyring(s) can be stored securely in an
  external secret management system such as Hashicorp Vault,
  AWS Secret Manager, etc. and have Ceph-CSI pull these
  sensitive keys directly from them.
- Provides additional security in comparison with K8s secrets,
  which are relatively easy to access and
  store sensitive secrets as unencrypted based64 encoded text.

### Drawback

Adds additional overhead as on each CSI RPC call
a request is made to KMS for fetching credentials.

## Background

### Credentials Management

Ceph-CSI relies on K8s CSI components for obtaining the Ceph user credentials.
K8s CSI uses the following reserved StorageClass parameters for fetching
credentials and passes them to Ceph-CSI RPC calls.

```yaml
csi.storage.k8s.io/controller-expand-secret-name
csi.storage.k8s.io/controller-expand-secret-namespace
csi.storage.k8s.io/provisioner-secret-name
csi.storage.k8s.io/provisioner-secret-namespace
csi.storage.k8s.io/node-stage-secret-name
csi.storage.k8s.io/node-stage-secret-namespace
```

### KMS Implementation

Ceph-CSI utilizes the `encryptionKMSID` StorageClass parameter and
corresponding config entry in KMS ConfigMap
(eg. [ceph-csi-encryption-kms-config](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml))
to integrate with the KMS.

## Extending existing implementation

> ### Deviation from existing approach
>
> - *StorageClass* parameters, such as `encryptionKMSID`,
> cannot be used for credentials as
> they are not passed in all CSI requests e.g. DeleteVolumeRequest.
> - Similarly, using K8s namespace as tenant name for KMS wouldn’t
> be possible as this information not included as part of all CSI requests.

- KMS ID is expected to be present as part of provisioner,
  node-stage, controller-expand secrets, along with Ceph userID.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-rbd-provisioner
stringData:
  userID: <ceph-user-id>
  kmsID: <kms-id>
 ```

- A new ConfigMap named ceph-csi-creds-kms-config similar to
  [ceph-csi-encryption-kms-config](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml)
  will be added.

```yaml
apiVersion: v1
kind: ConfigMap
data:
    config.json: |-
        {<kms-id>: <config>}
metadata:
    name: ceph-csi-creds-kms-config
```

- For KMS that rely on Tenant namespace, a new optional field name "tenantNamespace"
  is added in addition to existing KMS config.
  If "tenantNamespace" is not provided, Ceph-CSI namespace will be used to lookup
  required ServiceAccount, ConfigMap or Secrets.

**Example KMS config:**

```json
    "vault-tenant-sa-test": {
        "credsKMSType": "vaulttenantsa",
        "vaultAddress": "http://vault.default.svc.cluster.local:8200",
        "vaultBackend": "kv-v2",
        "vaultBackendPath": "shared-secrets",
        "vaultDestroyKeys": "false",
        "vaultTLSServerName": "vault.default.svc.cluster.local",
        "vaultCAVerify": "false",
        "tenantConfigName": "ceph-csi-kms-config",
        "tenantSAName": "ceph-csi-vault-sa",
        "tenantNamespace": "any-tenant"
    }
```

- Credential KMS config wouldn’t support nested tenant config
  provided using the `tenant` field,
  which implies each tenant is expected to have their own entry in the KMS ConfigMap.

### Backward Compatibility

KMS integration will only be enabled when `ceph-csi-creds-kms-config` ConfigMap exists
and secrets contain the `kmsID` key. In case where secrets contain
both `kmsID` and `userKey`
the keyring provided secrets will be used for creating the credential object.

### Integration with Rook

Rook integration for KMS support for credentials would be similar
PVC encryption.
A new option `CSI_ENABLE_KMS_CREDS` will be added to Rook operator.
When it is set to `true`, Rook will create
Ceph-CSI Deployments and DaemonSets that mount the Creds KMS ConfigMap.

At the time of writing this proposal, Rook only
supports storing Ceph keyring as secrets.
This [issue](https://github.com/rook/rook/issues/6374)
tracks adding support for storing
all Rook secrets in Vault. Till there is progress on this ticket,
it is assumed that an external script will be
responsible for creating the Ceph user and putting the keyring
in a KMS. Rook will only deploy Ceph-CSI with correct config.

## Code Changes

- New interface named CredStore will be introduced

```go
// CredStore allows KMS instances to implement a modular backend for Creds
// storage.
type CredStore interface {
    // FetchCreds reads the Creds from the configured store
    FetchCreds(userID string) (map[string]string, error)
}
```

- Provider Initializers will be refactored, if required,
  to accommodate both volume encryption and credential storage requirement.
