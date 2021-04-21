#!/bin/bash
set -e

# This script will be used by travis to run functional test
# against different Kubernetes version
export KUBE_VERSION=$1
shift
# parse the Kubernetes version, return the digit passed as argument
# v1.17.0 -> kube_version 1 -> 1
# v1.17.0 -> kube_version 2 -> 17
kube_version() {
    echo "${KUBE_VERSION}" | sed 's/^v//' | cut -d'.' -f"${1}"
}

# configure global environment variables
# shellcheck source=build.env
source "$(dirname "${0}")/../build.env"
cat << EOF | sudo tee -a /etc/environment
MINIKUBE_VERSION=${MINIKUBE_VERSION}
VM_DRIVER=${VM_DRIVER}
CHANGE_MINIKUBE_NONE_USER=${CHANGE_MINIKUBE_NONE_USER}
EOF

sudo scripts/minikube.sh up
sudo scripts/minikube.sh deploy-rook
sudo scripts/minikube.sh create-block-pool
# pull docker images to speed up e2e
sudo scripts/minikube.sh cephcsi
sudo scripts/minikube.sh k8s-sidecar
sudo chown -R travis: "$HOME"/.minikube /usr/local/bin/kubectl
KUBE_MAJOR=$(kube_version 1)
KUBE_MINOR=$(kube_version 2)
# skip snapshot operation if kube version is less than 1.17.0
if [[ "${KUBE_MAJOR}" -ge 1 ]] && [[ "${KUBE_MINOR}" -ge 17 ]]; then
    # delete snapshot CRD created by ceph-csi in rook
    scripts/install-snapshot.sh delete-crd
    # install snapshot controller
    scripts/install-snapshot.sh install
fi

# functional tests
make run-e2e E2E_ARGS="${*}"

if [[ "${KUBE_MAJOR}" -ge 1 ]] && [[ "${KUBE_MINOR}" -ge 17 ]]; then
    # delete snapshot CRD
    scripts/install-snapshot.sh cleanup
fi
sudo scripts/minikube.sh clean
