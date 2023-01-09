#!/bin/sh
#
# Create the Ceph-CSI ConfigMap based on the configuration obtained from the
# Rook deployment.
#
# The ConfigMap is referenced in the StorageClasses that are used by
# driver-*.yaml manifests in the k8s-e2e-external-storage CI job.
#
# Requirements:
# - kubectl in the path
# - working KUBE_CONFIG either in environment, or default config files
# - deployment done with Rook
#

# the namespace where Ceph-CSI is running
NAMESPACE="${1}"
[ -n "${NAMESPACE}" ] || { echo "ERROR: no namespace passed to ${0}"; exit 1; }

# exit on error
set -e

TOOLBOX_POD=$(kubectl -n rook-ceph get pods -l app=rook-ceph-tools -o=jsonpath='{.items[0].metadata.name}')
FS_ID=$(kubectl -n rook-ceph exec "${TOOLBOX_POD}" -- ceph fsid)
MONITOR=$(kubectl -n rook-ceph get services -l app=rook-ceph-mon -o=jsonpath='{.items[0].spec.clusterIP}:{.items[0].spec.ports[0].port}')

# in case the ConfigMap already exists, remove it before recreating
kubectl -n "${NAMESPACE}" delete --ignore-not-found configmap/ceph-csi-config

cat << EOF | kubectl -n "${NAMESPACE}" create -f -
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ceph-csi-config
data:
  config.json: |-
    [
      {
        "clusterID": "${FS_ID}",
        "monitors": [
          "${MONITOR}"
        ]
      }
    ]
EOF
