#!/bin/bash
set -e

# This script will be used by travis to run functional test
# against different kuberentes version
export KUBE_VERSION=$1
sudo scripts/minikube.sh up
sudo scripts/minikube.sh deploy-rook
# pull docker images to speed up e2e
sudo scripts/minikube.sh cephcsi
sudo scripts/minikube.sh k8s-sidecar
sudo chown -R travis: "$HOME"/.minikube /usr/local/bin/kubectl
# functional tests
go test github.com/ceph/ceph-csi/e2e --deploy-timeout=10 -timeout=30m --cephcsi-namespace=cephcsi-e2e-$RANDOM -v

sudo scripts/minikube.sh clean
