#!/bin/bash

set -o pipefail

if [[ -x "$(command -v gometalinter)" ]]; then
  gometalinter -j "${GO_METALINTER_THREADS:-1}" \
    --sort path --sort line --sort column --deadline=10m \
    --enable=misspell --enable=staticcheck \
    --vendor "${@-./...}"
else
  echo "WARNING: gometalinter not found, skipping lint tests" >&2
fi
