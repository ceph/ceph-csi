#!/bin/bash -e

# This script can be used to install/delete snapshotcontroller and snapshot beta CRD

SNAPSHOT_VERSION=${SNAPSHOT_VERSION:-"v2.1.1"}

SCRIPT_DIR="$(dirname "${0}")"

TEMP_DIR="$(mktemp -d)"
SNAPSHOTTER_URL="https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOT_VERSION}"

# controller
SNAPSHOT_RBAC="${SNAPSHOTTER_URL}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
SNAPSHOT_CONTROLLER="${SNAPSHOTTER_URL}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"

# snapshot CRD
SNAPSHOTCLASS="${SNAPSHOTTER_URL}/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
VOLUME_SNAPSHOT_CONTENT="${SNAPSHOTTER_URL}/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
VOLUME_SNAPSHOT="${SNAPSHOTTER_URL}/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"

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
    snapshotter_psp="${SCRIPT_DIR}/snapshot-controller-psp.yaml"
    mkdir -p "${TEMP_DIR}"
    curl -o "${temp_rbac}" "${SNAPSHOT_RBAC}"
    sed -i "s/namespace: default/namespace: ${namespace}/g" "${temp_rbac}"
    sed -i "s/namespace: default/namespace: ${namespace}/g" "${snapshotter_psp}"

    kubectl "${operation}" -f "${temp_rbac}"
    kubectl "${operation}" -f "${snapshotter_psp}"
    kubectl "${operation}" -f "${SNAPSHOT_CONTROLLER}" -n "${namespace}"
    kubectl "${operation}" -f "${SNAPSHOTCLASS}"
    kubectl "${operation}" -f "${VOLUME_SNAPSHOT_CONTENT}"
    kubectl "${operation}" -f "${VOLUME_SNAPSHOT}"
}

function delete_snapshot_crd() {
    kubectl delete -f "${SNAPSHOTCLASS}" --ignore-not-found
    kubectl delete -f "${VOLUME_SNAPSHOT_CONTENT}" --ignore-not-found
    kubectl delete -f "${VOLUME_SNAPSHOT}" --ignore-not-found
}

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
