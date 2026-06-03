#!/bin/sh
#
# Create StorageClasses from a template (sc-*.yaml.in) and replace keywords
# like @@CLUSTER_ID@@.
#
# These StorageClasses can then be used by driver-*.yaml manifests in the
# k8s-e2e-external-storage CI job.
#
# Requirements:
# - kubectl in the path
# - working KUBE_CONFIG either in environment, or default config files
# - deployment done with Rook
#

# exit on error
set -e

WORKDIR=$(dirname "${0}")

TOOLBOX_POD=$(kubectl -n rook-ceph get pods --no-headers -l app=rook-ceph-tools -o=jsonpath='{.items[0].metadata.name}')
FS_ID=$(kubectl -n rook-ceph exec "${TOOLBOX_POD}" -- ceph fsid)

for sc in "${WORKDIR}"/sc-*.yaml.in
do
	sed "s/@@CLUSTER_ID@@/${FS_ID}/" "${sc}" |
		kubectl create -f -
done
