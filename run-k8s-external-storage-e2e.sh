#!/bin/sh
#
# Download the Kubernetes test suite, extract it and run the external-storage suite.
#
# Requirements:
# - KUBE_VERSION needs to be set in the environment (format: "v1.18.5")

# exit on failure
set -e

[ -n "${KUBE_VERSION}" ] || { echo "KUBE_VERSION not set" ; exit 1 ; }

# download and extract the tests
curl -LO "https://storage.googleapis.com/kubernetes-release/release/${KUBE_VERSION}/kubernetes-test-linux-amd64.tar.gz"
tar xzf kubernetes-test-linux-amd64.tar.gz kubernetes/test/bin/ginkgo kubernetes/test/bin/e2e.test

# e2e depends on a self-contained KUBECONFIG for some reason
KUBECONFIG_TMP="$(mktemp -t kubeconfig.XXXXXXXX)"
kubectl config view --raw --flatten > "${KUBECONFIG_TMP}"
export KUBECONFIG="${KUBECONFIG_TMP}"

for driver in /opt/build/go/src/github.com/ceph/ceph-csi/scripts/k8s-storage/driver-*.yaml
do
	kubernetes/test/bin/ginkgo \
		-focus="External.Storage.*.csi.ceph.com" \
		-skip='\[Feature:|\[Disruptive\]' \
		kubernetes/test/bin/e2e.test \
		-- \
		-storage.testdriver="${driver}"
done
