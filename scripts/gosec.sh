#!/bin/bash

set -o pipefail

if [[ -x "$(command -v gosec)" ]]; then
  # gosec does not support -mod=vendor, so fallback to non-module support and
  # assume all dependencies are available in ./vendor already
  export GO111MODULE=off
  find cmd internal -type d -print0 | xargs --null gosec
else
  echo "WARNING: gosec not found, skipping security tests" >&2
fi
