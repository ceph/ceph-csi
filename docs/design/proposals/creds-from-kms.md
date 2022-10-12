# Fetching Ceph user credentials from KMS

## Proposal

Ceph-CSI supports several KMS implementations for storing and
fetching DEKs (Data Encryption Keys) used for RBD volume encryption.
The focus of this proposal is to leverage existing KMS implementation
to support obtaining Ceph user keyring used while
creating, deleting, mapping and resizing volumes.

### Benefits

- For scenarios where Ceph is deployed externally, either as
  a standalone or through Rook, one needs to obtain the Ceph keyring(s)
  and manually create the K8s secrets needed by Ceph-CSI. With this feature,
  sensitive keyring(s) can be stored securely in an
  external secret management system such as Hashicorp Vault,
  AWS Secret Manager, etc., and have Ceph-CSI pull these
  keys directly from them.
- More secure compared to K8s secrets,
  which are relatively easy to access and
  store sensitive secrets as unencrypted based64 encoded text.

### Drawbacks

- Adds additional overhead as on each CSI RPC call
  a request is made to KMS for fetching credentials.
- Possible risk of hitting the KMS rate limit.

## Extending existing implementation

- KMS ID is provided as part of the provisioner,
  node-stage, and controller-expand secrets, along with Ceph `userID`, as shown below.
  Having this information as part of *StorageClass* parameters,
  like using the `encryptionKMSID` key for volume encryption,
  wouldn't work as these parameters are not passed
  to all CSI RPCs, e.g. `DeleteVolume`, `ControllerExpand` etc.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-rbd-provisioner
stringData:
  userID: <ceph-user-id>
  kmsID: <kms-id>
 ```

- KMS config entry corresponding to `kmsID` needs to be added to existing
  KMS ConfigMap, very similar to the volume encryption config found in
  [cph-csi-encryption-kms-config](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml).

- For KMS that rely on tenants' namespace to obtain required
  ServiceAccount, ConfigMap, or Secrets,
  an optional field, "tenantNamespace" will be added to the KMS config.
  As the tenant namespace isn't available in all CSI requests,
  value from provided in this field will be used instead.
  If "tenantNamespace" field is absent, Ceph-CSI namespace
  will be used as default.

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
and CSI secrets contain the `kmsID` key. In case where secrets contain
both `kmsID` and `userKey`
the keyring provided in the secret will be used for creating the credential object.

### Integration with Rook

Rook integration for KMS support for credentials would be similar
PVC encryption.
A new option `CSI_ENABLE_KMS_CREDS` will be added to Rook operator.
When it is set to `true`, Rook will create
Ceph-CSI Deployments and DaemonSets that mount the KMS ConfigMap.

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
