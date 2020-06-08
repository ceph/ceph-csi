#!/bin/bash

# generating secret example
# ---
# apiVersion: v1
# kind: Secret
# metadata:
#   name: csi-rbd-secret
#   namespace: ceph
# stringData:
#   # Key values correspond to a user name and its key, as defined in the
#   # ceph cluster. User ID should have required access to the 'pool'
#   # specified in the storage class
#   userID: admin
#   userKey: AQDNHdVeAAAAABAA7J6XIrtukvQDaEZ/xCs+Rg==
# get USERID, NAMESPACE, CEPH_CSI_SECRET on ENV

set -x
ENCODETE_KEYRING=$(kubectl get secret -n ceph ceph-client-admin-keyring -o jsonpath='{.data.ceph\.client\.admin\.keyring}' | base64 -d | grep key | awk '{print $NF}')
SECRET=$(mktemp --suffix .yaml)

function cleanup {
  rm -f ${SECRET}
}
trap cleanup EXIT

cat > ${SECRET} <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: "${CEPH_CSI_SECRET}"
type: kubernetes.io/rbd
data:
  userID: $( echo ${USERID} )
  userKey: $( echo ${ENCODED_KEYRING} )
EOF

kubectl apply --namespace ${NAMESPACE} -f ${SECRET}
