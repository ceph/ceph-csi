# v3.18 Pending Release Notes

## Breaking changes

- Journal metadata now uses sharded `csiDirectory` objects for new CSI name
  mappings. This is a forward-only upgrade for journal metadata: newer
  ceph-csi versions can read legacy `csiDirectory` entries, but older versions
  only read the legacy unsharded object and cannot see mappings written to
  shard oids after the upgrade. Downgrading after new entries have been written
  in the upgraded version can break idempotent lookups for those entries.

## Features

## NOTE
