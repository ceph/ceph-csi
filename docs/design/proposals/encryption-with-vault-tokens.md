# Multi-tenancy with Vault Tokens

## Current Feature: Vault access with single token

The current feature to [support Hashicorp Vault as a KMS is
documented](./encrypted-pvc.md):

- Tenants can create PVCs that are encrypted with a unique key (Data Encryption
  Key, DEK)
- the key to encrypt/decrypt the PVC is stored in Hashicorp Vault
- there is a single JSON Web Token (JWT), the Kubernetes ServiceAccount, to
  access Vault
- enabling is done in the StorageClass, pointing to a KMS configuration section
  in a JSON configuration file (Kubernetes ConfigMap)

## High-Level Requirements

The new feature should support multi-tenancy, so that each Tenant can use their
own Vault Token to access the Key Encryption Key (KEK) and fetch the Data
Encryption Key (DEK) for PVC encryption:

- each tenant can manage their Vault Token
- to get the KEK from Vault, each Tenant should provide their personal Vault token
- Ceph-CSI should use the Vault Token from the Tenant to store the unique key
  (DEK) for the PVC
- a Tenant is the owner of a Kubernetes Namespace

## Restrictions to consider

- Tenants can only configure their Kubernetes namespace, their token to access
  Vault should be located in their namespace (as a Kubernetes Secret)
- Ceph-CSI can not directly access Kubernetes namespaces, as CSI is an
  abstraction layer
- Ceph-CSI needs to talk to a service (sidecar) to access the Vault Token from
  a Tenant
- the KMS configuration is a ConfigMap with name
  `ceph-csi-encryption-kms-config` in the namespace where the Ceph-CSI pods are
  running
- the KMS configuration is available for the Ceph-CSI pods at
  `/etc/ceph-csi-encryption-kms-config/config.json`
- [example of the
  configuration](https://github.com/ceph/ceph-csi/blob/devel/examples/kms/vault/kms-config.yaml)

## Dependencies

- the name of the Kubernetes Secret needs to be known (configured in the KMS
  config file)
- each Tenant (Kubernetes Namespace) should use the same name for the Secret
  that contains the Vault token
- the CSI-provisioner sidecar needs to provide the name of the Tenant together
  with the metadata of the PVC to the Ceph-CSI provisioner

## Implementation Outline

- when creating the PVC the Ceph-CSI provisioner needs to store the Kubernetes
  Namespace of the PVC in its metadata
   - stores the `csi.volume.owner` (name of Tenant) in the metadata of the
    volume and sets it as `rbdVolume.Owner`
- the Ceph-CSI node-plugin needs to request the Vault Token in the NodeStage
  CSI operation and create/get the key for the PVC
- the Ceph-CSI provisioner needs to request the Vault Token to delete the key
  for the PVC in the VolumeDelete CSI operation

### New sidecar: Fetching Tenant Tokens from Kubernetes

CSI is an abstraction and should not communicate with Kubernetes or other
Container Orchestration systems directly. Therefore, it is needed to
communicate with a service that can provide the Vault Token from the Tenants.
This service can be provided by a sidecar, exposing an endpoint as a
UNIX-Domain-Socket to communicate through.

This sidecar does not exist at the moment. There are other features within
Ceph-CSI that will benefit from this sidecar, e.g. Topology support. Because
Ceph-CSI already uses the Kubernetes API to fetch details from Kubernetes, it
should be acceptable to fetch the Vault Token configuration for the Tenants the
same way.

The feature for a sidecar that provides access to the required information from
Kubernetes and other Container Orchestration frameworks is tracked in
[#1782](https://github.com/ceph/ceph-csi/issues/1782).

## Configuration Details

- the current KMS configuration file needs extensions for Vault Token support
- introduce a new KMS-type: VaultTokenKMS (the current VaultKMS uses a
  Kubernetes ServiceAccount)
- configuration of the VaultTokenKMS can be very similar to VaultKMS for common
  settings
- the configuration can override the defaults for each Tenant separately
   - Vault Service connection details (address, TLS options, ...)
   - name of the Kubernetes Secret that can be looked up per tenant
- the configuration points to a Kubernetes Secret per Tenant that contains the
  Vault Token
- the configuration points to an optional Kubernetes ConfigMap per Tenant that
  contains alternative connection options for the VaultTokenKMS service

### Example of the KMS configuration file for Vault Tokens

The configuration is available in the Ceph-CSI containers as
`/etc/ceph-csi-encryption-kms-config/config.json`:

```json
{
    "vault-with-tokens": {
        "encryptionKMSType": "vaulttokens",
        "vaultAddress": "http://vault.default.svc.cluster.local:8200",
        "vaultBackendPath": "secret/",
        "vaultTLSServerName": "vault.default.svc.cluster.local",
        "vaultCAFromSecret": "vault-ca",
        "vaultCAVerify": "false",
        "tenantConfigName": "ceph-csi-kms-config",
        "tenantTokenName": "ceph-csi-kms-token",
        "tenants": {
            "my-app": {
                "vaultAddress": "https://vault.example.com",
                "vaultCAVerify": "true"
            },
            "an-other-app": {
                "tenantTokenName": "storage-encryption-token"
            }
        }
    }
}
```

In the Kubernetes StorageClass, the `kmsID` should be set to
`vault-with-tokens` in order to select the above configuration.

**Required options**:

- `encryptionKMSType`: should be set to `vaulttokens`
- `vaultAddress`: should be set to the URL of the Vault service

**Optional options**:

- `vaultBackendPath`: defaults to `secret/`
- `vaultTLSServerName`: not used when unset
- `vaultCAFromSecret`: not used when unset
- `vaultCAVerify`: defaults to `true`
- `tenantConfigName`: the name of the Kubernetes ConfigMap that contains the
  Vault connection configuration (can be overridden per Tenant, defaults to
  `ceph-csi-kms-config`)
- `tenantTokenName`: the name of the Kubernetes Secret that contains the Vault
  Token (can be overridden per Tenant, defaults to `ceph-csi-kms-token`)
- `tenants`: list of Tenants (Kubernetes Namespaces) with their connection
  configuration that differs from the global parameters

### Configuration stored in the Tenants Kubernetes Namespace

The Vault Token needs to be configured per Tenant. Each Tenant can create,
modify or delete their own personal Token. The Token is stored in a Kubernetes
Secret in the Kubernetes Namespace where the PVC is created.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: ceph-csi-kms-token
stringData:
  token: "sample_root_token_id"
```

The name `ceph-csi-kms-token` is the default, but can be changed by setting
`tenantTokenName` in the `/etc/ceph-csi-encryption-kms-config/config.json`
configuration file.

In addition to the Vault Token that can be configured per Tenant, the
connection parameters to the Vault Service can be stored in the Tenants
Kubernetes Namespace as well.

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ceph-csi-kms-config
data:
  vaultAddress: "https://vault.infosec.example.org"
  vaultBackendPath: "secret/ceph-csi-encryption/"
  vaultTLSServerName: "vault.infosec.example.org"
  vaultCAFromSecret: "vault-infosec-ca"
  vaultClientCertFromSecret: "vault-client-cert"
  vaultClientCertKeyFromSecret: "vault-client-cert-key"
  vaultCAVerify: "true"
```

Only parameters with the `vault`-prefix may be changed in the Kubernetes
ConfigMap of the Tenant.

### Certificates stored in the Tenants Kubernetes Namespace

The `vaultCAFromSecret` , `vaultClientCertFromSecret` and
`vaultClientCertKeyFromSecret` secrets should be created in the namespace where
Ceph-CSI is deployed. The sample of secrets for the CA and client Certificate.

#### CA Certificate to verify Vault server TLS certificate

```yaml
---
apiVersion: v1
kind: secret
metadata:
  name: vault-infosec-ca
stringData:
  cert: |
    MIIC2DCCAcCgAwIBAgIBATANBgkqh...
```

#### Client Certificate for Vault connection

```yaml
---
apiVersion: v1
kind: secret
metadata:
  name: vault-client-cert
stringData:
  cert: |
    BATANBgkqcCgAwIBAgIBATANBAwI...
```

#### Client Certificate key for Vault connection

```yaml
---
apiVersion: v1
kind: secret
metadata:
  name: vault-client-cert-key
stringData:
  key: |
    KNSC2DVVXcCgkqcCgAwIBAgIwewrvx...
```

It is also possible for a user to create a single Secret that contains both
the client authentication and update the configuration to fetch the certificate
and key from the Secret.

```yaml
---
apiVersion: v1
kind: secret
metadata:
  name: vault-client-auth
stringData:
  cert: |
    MIIC2DCCAcCgAwIBAgIBATANBgkqh...
  key: |
    KNSC2DVVXcCgkqcCgAwIBAgIwewrvx...
```
