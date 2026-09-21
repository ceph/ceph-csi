# v3.19 Pending Release Notes

## Breaking changes

## Features

1. Added an optional `TLS_MIN_VERSION` setting to the `kmip` KMS provider,
   which sets the minimum TLS version the provider may use. That minimum was
   previously hardcoded to TLS 1.2, and TLS 1.3 was used whenever the KMIP
   server offered it, with no way to require it. The setting defaults to
   `"1.2"` and keeps that behavior. Setting it to `"1.3"` requires TLS 1.3, so
   that a deployment held to a TLS 1.3 baseline fails to connect to a KMIP
   server that only offers TLS 1.2 instead of silently continuing over
   TLS 1.2. The negotiated TLS version and cipher suite are now logged for
   each KMIP connection.

## NOTE
