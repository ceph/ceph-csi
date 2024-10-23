#!/bin/bash -E

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck disable=SC1091
[ ! -e "${SCRIPT_DIR}"/utils.sh ] || source "${SCRIPT_DIR}"/utils.sh

# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../build.env"

CEPH_CSI_OPERATOR_VERSION=${CEPH_CSI_OPERATOR_VERSION:-"latest"}
# Override version to "main" if it is "latest"
if [ "$CEPH_CSI_OPERATOR_VERSION" = "latest" ]; then
    CEPH_CSI_OPERATOR_VERSION="main"
fi
OPERATOR_URL="https://raw.githubusercontent.com/ceph/ceph-csi-operator/${CEPH_CSI_OPERATOR_VERSION}"

# operator deployment files
OPERATOR_INSTALL="${OPERATOR_URL}/deploy/all-in-one/install.yaml"

OPERATOR_NAMESPACE="ceph-csi-operator-system"
IMAGESET_CONFIGMAP_NAME="ceph-csi-imageset"
ENCRYPTION_CONFIGMAP_NAME="ceph-csi-encryption-kms-config"

# csi drivers
RBD_DRIVER_NAME="rbd.csi.ceph.com"
CEPHFS_DRIVER_NAME="cephfs.csi.ceph.com"
NFS_DRIVER_NAME="nfs.csi.ceph.com"

# k8s csi sidecar image
K8S_IMAGE_REPO=${K8S_IMAGE_REPO:-"registry.k8s.io/sig-storage"}

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

function generate_imageset_configmap() {
    cat <<EOF >"${TEMP_DIR}/imageset-configmap.yaml"
apiVersion: v1
kind: ConfigMap
metadata:
    name: ${IMAGESET_CONFIGMAP_NAME}
    namespace: ${OPERATOR_NAMESPACE}
data:
    "plugin": "quay.io/cephcsi/cephcsi:${CSI_IMAGE_VERSION}"  # test image
    "attacher": "${K8S_IMAGE_REPO}/csi-attacher:${CSI_ATTACHER_VERSION}"
    "snapshotter": "${K8S_IMAGE_REPO}/csi-snapshotter:${CSI_SNAPSHOTTER_VERSION}"
    "provisioner": "${K8S_IMAGE_REPO}/csi-provisioner:${CSI_PROVISIONER_VERSION}"
    "registrar": "${K8S_IMAGE_REPO}/csi-node-driver-registrar:${CSI_NODE_DRIVER_REGISTRAR_VERSION}"
    "resizer": "${K8S_IMAGE_REPO}/csi-resizer:${CSI_RESIZER_VERSION}"
EOF
}

function generate_encryption_configmap() {
    cat <<EOF >"${TEMP_DIR}/encryption-configmap.yaml"
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: ${OPERATOR_NAMESPACE}
  name: ${ENCRYPTION_CONFIGMAP_NAME}
data:
    config.json: ""
EOF
}

function generate_operator_config() {
    generate_imageset_configmap
    generate_encryption_configmap

    cat <<EOF >"${TEMP_DIR}/operatorconfig.yaml"
apiVersion: csi.ceph.io/v1alpha1
kind: OperatorConfig
metadata:
    name: ceph-csi-operator-config
    namespace: ${OPERATOR_NAMESPACE}
spec:
    driverSpecDefaults:
        snapshotPolicy: volumeGroupSnapshot
        controllerPlugin:
            deploymentStrategy:
                type: Recreate
        generateOMapInfo: true
        enableMetadata: true
        log:
            verbosity: 5 # csi pods log level
        imageSet:
            name: ${IMAGESET_CONFIGMAP_NAME}
        encryption:
            configMapName:
                name: ${ENCRYPTION_CONFIGMAP_NAME}
    log:
        verbosity: 3 # operator log level
EOF
}

function generate_driver() {
    local driver_name=$1
    cat <<EOF >"${TEMP_DIR}/${driver_name}.yaml"
apiVersion: csi.ceph.io/v1alpha1
kind: Driver
metadata:
  name: ${driver_name}
  namespace: ${OPERATOR_NAMESPACE}
EOF
}

function deploy_operator() {
    kubectl_retry create -f "${OPERATOR_INSTALL}"
    generate_operator_config
    generate_driver "${RBD_DRIVER_NAME}"
    generate_driver "${CEPHFS_DRIVER_NAME}"
    generate_driver "${NFS_DRIVER_NAME}"

    # Display the contents of the generated files for debugging
    for file in "${TEMP_DIR}"/*; do
        cat "$file"
        echo
    done

    # Apply all the generated files at once
    kubectl_retry create -f "${TEMP_DIR}"
}

function cleanup() {
    generate_driver "${RBD_DRIVER_NAME}"
    generate_driver "${CEPHFS_DRIVER_NAME}"
    generate_driver "${NFS_DRIVER_NAME}"
    generate_operator_config

    # Delete all the generated files at once
    kubectl_retry delete -f "${TEMP_DIR}"
    kubectl_retry delete -f "${OPERATOR_INSTALL}"
}

case "${1:-}" in
deploy)
    deploy_operator
    ;;
cleanup)
    cleanup
    ;;
*)
    echo "Usage:" >&2
    echo "  $0 deploy" >&2
    echo "  $0 cleanup" >&2
    exit 1
    ;;
esac
