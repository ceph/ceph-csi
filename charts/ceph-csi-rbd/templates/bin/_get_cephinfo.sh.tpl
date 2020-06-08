#!/bin/bash


# generating cm example
# ---
# apiVersion: v1
# kind: ConfigMap
# data:
#   config.json: |-
#     [
#       {
#         "clusterID": "d52a9d00-64b9-45f0-b564-08dffe95f847",
#         "monitors": [
#           "ceph-mon.ceph.svc.cluster.local:6789"
#         ]
#       }
#     ]
# metadata:
#   name: ceph-csi-config-rbd
#   namespace: ceph

# get NAMESPACE, CEPH_CSI_CONFIGMAP on ENV
set -x
CLUSTERID=$(kubectl get cm -n ceph ceph-client-etc -o jsonpath="{.data.ceph\.conf}" | grep fsid | awk '{print $NF}')
MONITORS=$(kubectl get cm -n ceph ceph-client-etc -o jsonpath="{.data.ceph\.conf}" | grep mon_host | awk '{print $NF}')
CONFIGMAP=$(mktemp --suffix .yaml)

function cleanup {
  rm -f ${CONFIGMAP}
}
trap cleanup EXIT

cat > ${CONFIGMAP} <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: "${CEPH_CSI_CONFIGMAP}"
data:
  config.json: |-
    [
      {
        "clusterID": "$( echo ${CLUSTERID} )",
        "monitors": [
            "$( echo ${MONITORS} )"
        ]
      }
    ]
EOF

kubectl apply --namespace ${NAMESPACE} -f ${CONFIGMAP}
