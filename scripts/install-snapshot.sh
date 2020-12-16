#!/bin/bash -e

# This script can be used to install/delete snapshotcontroller and snapshot beta CRD

SCRIPT_DIR="$(dirname "${0}")"

# shellcheck source=build.env
source "${SCRIPT_DIR}/../build.env"

SNAPSHOT_VERSION=${SNAPSHOT_VERSION:-"v3.0.1"}

TEMP_DIR="$(mktemp -d)"
SNAPSHOTTER_URL="https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOT_VERSION}"

# controller
SNAPSHOT_RBAC="${SNAPSHOTTER_URL}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
SNAPSHOT_CONTROLLER="${SNAPSHOTTER_URL}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"

# snapshot CRD
SNAPSHOTCLASS="${SNAPSHOTTER_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
VOLUME_SNAPSHOT_CONTENT="${SNAPSHOTTER_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
VOLUME_SNAPSHOT="${SNAPSHOTTER_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"

function install_snapshot_controller() {
    local namespace=$1
    if [ -z "${namespace}" ]; then
        namespace="default"
    fi

    create_or_delete_resource "create" ${namespace}

    pod_ready=$(kubectl get pods -l app=snapshot-controller -n ${namespace} -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
    INC=0
    until [[ "${pod_ready}" == "true" || $INC -gt 20 ]]; do
        sleep 10
        ((++INC))
        pod_ready=$(kubectl get pods -l app=snapshot-controller -n ${namespace} -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
        echo "snapshotter pod status: ${pod_ready}"
    done

    if [ "${pod_ready}" != "true" ]; then
        echo "snapshotter controller creation failed"
        kubectl get pods -l app=snapshot-controller -n ${namespace}
        kubectl describe po -l app=snapshot-controller -n ${namespace}
        exit 1
    fi

    echo "snapshot controller creation successful"
}

function cleanup_snapshot_controller() {
    local namespace=$1
    if [ -z "${namespace}" ]; then
        namespace="default"
    fi
    create_or_delete_resource "delete" ${namespace}
}

function create_or_delete_resource() {
    local operation=$1
    local namespace=$2
    temp_rbac=${TEMP_DIR}/snapshot-rbac.yaml
    temp_snap_controller=${TEMP_DIR}/snapshot-controller.yaml
    snapshotter_psp="${SCRIPT_DIR}/snapshot-controller-psp.yaml"
    mkdir -p "${TEMP_DIR}"
    curl -o "${temp_rbac}" "${SNAPSHOT_RBAC}"
    curl -o "${temp_snap_controller}" "${SNAPSHOT_CONTROLLER}"
    sed -i "s/namespace: default/namespace: ${namespace}/g" "${temp_rbac}"
    sed -i "s/namespace: default/namespace: ${namespace}/g" "${snapshotter_psp}"
    sed -i "s/canary/${SNAPSHOT_VERSION}/g" "${temp_snap_controller}"

    kubectl "${operation}" -f "${temp_rbac}"
    kubectl "${operation}" -f "${snapshotter_psp}"
    kubectl "${operation}" -f "${temp_snap_controller}" -n "${namespace}"
    kubectl "${operation}" -f "${SNAPSHOTCLASS}"
    kubectl "${operation}" -f "${VOLUME_SNAPSHOT_CONTENT}"
    kubectl "${operation}" -f "${VOLUME_SNAPSHOT}"
}

function delete_snapshot_crd() {
    kubectl delete -f "${SNAPSHOTCLASS}" --ignore-not-found
    kubectl delete -f "${VOLUME_SNAPSHOT_CONTENT}" --ignore-not-found
    kubectl delete -f "${VOLUME_SNAPSHOT}" --ignore-not-found
}

# parse the kubernetes version
# v1.17.2 -> kube_version 1 -> 1  (Major)
# v1.17.2 -> kube_version 2 -> 17 (Minor)
function kube_version() {
    echo "${KUBE_VERSION}" | sed 's/^v//' | cut -d'.' -f"${1}"
}

if ! get_kube_version=$(kubectl version --short) ||
   [[ -z "${get_kube_version}" ]]; then
    echo "could not get Kubernetes server version"
    echo "hint: check if you have specified the right host or port"
    exit 1
fi

KUBE_VERSION=$(echo "${get_kube_version}" | grep "^Server Version" | cut -d' ' -f3)
KUBE_MAJOR=$(kube_version 1)
KUBE_MINOR=$(kube_version 2)

# skip snapshot operation if kube version is less than 1.17.0
if [[ "${KUBE_MAJOR}" -lt 1 ]] || [[ "${KUBE_MAJOR}" -eq 1  &&  "${KUBE_MINOR}" -lt 17 ]]; then
    echo "skipping: Kubernetes server version is < 1.17.0"
    exit 1
fi

case "${1:-}" in
install)
    install_snapshot_controller "$2"
    ;;
cleanup)
    cleanup_snapshot_controller "$2"
    ;;
delete-crd)
    delete_snapshot_crd
    ;;
*)
    echo "usage:" >&2
    echo "  $0 install" >&2
    echo "  $0 cleanup" >&2
    echo "  $0 delete-crd" >&2
    ;;
esac
