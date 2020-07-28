#!/bin/bash

# Credits to keel-hq/keel for the idea:
# https://github.com/keel-hq/keel/blob/master/deployment/scripts/gen-deploy.sh

# Usage: ./gen-deploy.sh or ./gen-deploy.sh <cephfs/rbd>

set -o errexit
set -o nounset
set -o pipefail


cleanup () {
  rm -f "${OUTPUT_DIR}/*"
}

generate() {
  output_file="${1}"
  templates="${2}"

  # TODO: Helm template ignores the namespace, decide what to do with this
  # https://github.com/helm/helm/issues/3553
	helm template \
    "${CHART}" \
		"charts/${CHART}" \
		--namespace "ceph-csi" \
    --create-namespace \
    --show-only "${templates}" \
  > "${OUTPUT_DIR}/${output_file}"
}

plugins="cephfs rbd"
# If no parameter is passed, regenerate both
if [ -n "${1+x}" ]; then
  plugins="${1}"
fi

for plugin in ${plugins}; do
  CHART="ceph-csi-${plugin}"
  OUTPUT_DIR="deploy/${plugin}/kubernetes"

  # Remove everything
  cleanup

  echo "Generating ${CHART}"

  # Loop the templates
  for file in charts/"${CHART}"/templates/*.yaml; do
    # Get just the basename
    filename=${file##*/}

    # Skip crd until helm/helm#7295 is fixed
    # https://github.com/helm/helm/issues/7295
    if [ "$filename" = "csidriver-crd.yaml" ]; then
      continue
    fi

    # Generate the file
    generate "$filename" "templates/$filename"
  done
done
