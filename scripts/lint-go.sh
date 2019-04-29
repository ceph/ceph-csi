#!/bin/bash

set -o pipefail

if [[ -x "$(command -v golangci-lint)" ]]; then
  golangci-lint --config=scripts/golangci.yml run ./... -v
else
  echo "WARNING: golangci-lint not found, skipping lint tests" >&2
fi
