#!/bin/bash

set -o pipefail

if [[ -x "$(command -v gosec)" ]]; then
  find cmd pkg -type d -print0 | xargs --null gosec
else
  echo "WARNING: gosec not found, skipping security tests" >&2
fi
