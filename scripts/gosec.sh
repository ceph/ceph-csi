#!/bin/bash

set -o pipefail

if [[ -x "$(command -v gosec)" ]]; then
  find cmd internal -type d -print0 | xargs --null gosec "${GO_TAGS}"
else
  echo "WARNING: gosec not found, skipping security tests" >&2
fi
