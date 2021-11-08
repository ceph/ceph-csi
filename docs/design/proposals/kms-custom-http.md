# External Storage of Encryption Passphrases

## Goal

The current KMS methods are modeled to provide an end-to-end solution for
managing the encryption passphrases that are used by ceph-csi. While they
are fine and use top-notch services like Vault and Amazon KMS, they do
struggle to fit into configurations where the encryption is handleded
completely by a dedicated in-house service.

The goal is to allow configuring a very thin kms method that completely
externalizes the storage and retrieval of passphrases to an opaque service.

## Solution Requirements

1. For new volumes the generated passphrases should be sent to the service
   for persisting.
1. For existing volumes the passphrases should be fetched from the
   aftermentioned service.
1. The communication should be secure because the passphrases are transferred
   in plain format.

## Suggested Implementation

New KMS type: `opaqueservice`

Ultimately, we strongly suggest using mTLS for communicating with the endpoint.
However, the security level is up to the system operator to decide so we are
better off allowing several options.

Example configuration:

```json
{
  "mykms": {
    "encryptionKMSType": "opaqueservice",
    "serviceurl": "https://my.service.com/tokens",
    "mtls": "true",
    "basicauth": "false"
  }
}
```

The service is expected to respect these endpoints:

* Persist a new volume passphrase
  `POST /key`
  payload: `{ "volumeid": "vol123", "key": "<passphrase>" }`
* Retrieve an existing passphrase
  `GET /key/<volume-id>`
  response: `{"key":"<passphrase>"}`
* Delete a pasphrase
  `DELETE /key/<volume-id>`
