#!/bin/bash
set -e

# This script will be used by centos CI to run functional test
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
cat <<EOF | sudo tee -a /etc/environment
HELM_VERSION=${HELM_VERSION}
MINIKUBE_VERSION=${MINIKUBE_VERSION}
VM_DRIVER=${VM_DRIVER}
CHANGE_MINIKUBE_NONE_USER=${CHANGE_MINIKUBE_NONE_USER}
EOF

# helm is installed from this shell, not a new one that reads /etc/environment
export HELM_VERSION=${HELM_VERSION}

sudo scripts/minikube.sh up
sudo scripts/minikube.sh deploy-rook
sudo scripts/minikube.sh create-block-pool
# pull docker images to speed up e2e
sudo scripts/minikube.sh cephcsi
sudo scripts/minikube.sh k8s-sidecar

NAMESPACE=cephcsi-e2e-$RANDOM
# create ns for e2e
kubectl create ns ${NAMESPACE}
# install snapshot controller and create snapshot CRD
scripts/install-snapshot.sh install
# set up helm
scripts/install-helm.sh up
# install cephcsi helm charts
scripts/install-helm.sh install-cephcsi --namespace ${NAMESPACE}
# functional tests
make run-e2e NAMESPACE="${NAMESPACE}" E2E_ARGS="--deploy-cephfs=false --deploy-rbd=false ${*}"

# cleanup
scripts/install-snapshot.sh cleanup
scripts/install-helm.sh cleanup-cephcsi --namespace ${NAMESPACE}
scripts/install-helm.sh clean
kubectl delete ns ${NAMESPACE}
sudo scripts/minikube.sh clean
