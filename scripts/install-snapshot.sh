#!/bin/bash -e

# This script can be used to install/delete snapshotcontroller and snapshot CRD

SCRIPT_DIR="$(dirname "${0}")"

# shellcheck source=build.env
source "${SCRIPT_DIR}/../build.env"

# shellcheck disable=SC1091
[ ! -e "${SCRIPT_DIR}"/utils.sh ] || source "${SCRIPT_DIR}"/utils.sh

SNAPSHOT_VERSION=${SNAPSHOT_VERSION:-"v5.0.1"}

TEMP_DIR="$(mktemp -d)"
SNAPSHOTTER_URL="https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOT_VERSION}"

# controller
SNAPSHOT_RBAC="${SNAPSHOTTER_URL}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
SNAPSHOT_CONTROLLER="${SNAPSHOTTER_URL}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"

# snapshot CRD
SNAPSHOTCLASS="${SNAPSHOTTER_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
VOLUME_SNAPSHOT_CONTENT="${SNAPSHOTTER_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
VOLUME_SNAPSHOT="${SNAPSHOTTER_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"

# volumegroupsnapshot CRD
VOLUME_GROUP_SNAPSHOTCLASS="${SNAPSHOTTER_URL}/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotclasses.yaml"
VOLUME_GROUP_SNAPSHOT_CONTENT="${SNAPSHOTTER_URL}/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotcontents.yaml"
VOLUME_GROUP_SNAPSHOT="${SNAPSHOTTER_URL}/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshots.yaml"

function install_snapshot_controller() {
    local namespace=$1
    if [ -z "${namespace}" ]; then
        namespace="kube-system"
    fi

    create_or_delete_resource "create" "${namespace}"

    pod_ready=$(kubectl_retry get pods -l app.kubernetes.io/name=snapshot-controller -n "${namespace}" -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
    INC=0
    until [[ "${pod_ready}" == "true" || $INC -gt 20 ]]; do
        sleep 10
        ((++INC))
        pod_ready=$(kubectl_retry get pods -l app.kubernetes.io/name=snapshot-controller -n "${namespace}" -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
        echo "snapshotter pod status: ${pod_ready}"
    done

    if [ "${pod_ready}" != "true" ]; then
        echo "snapshotter controller creation failed"
        kubectl_retry get pods -l app.kubernetes.io/name=snapshot-controller -n "${namespace}"
        kubectl_retry describe po -l app.kubernetes.io/name=snapshot-controller -n "${namespace}"
        exit 1
    fi

    echo "snapshot controller creation successful"
}

function cleanup_snapshot_controller() {
    local namespace=$1
    if [ -z "${namespace}" ]; then
        namespace="kube-system"
    fi
    create_or_delete_resource "delete" "${namespace}"
}

function create_or_delete_resource() {
    local operation=$1
    local namespace=$2
    temp_rbac=${TEMP_DIR}/snapshot-rbac.yaml
    temp_snap_controller=${TEMP_DIR}/snapshot-controller.yaml
    mkdir -p "${TEMP_DIR}"
    curl -o "${temp_rbac}" "${SNAPSHOT_RBAC}"
    curl -o "${temp_snap_controller}" "${SNAPSHOT_CONTROLLER}"
    sed -i "s/namespace: kube-system/namespace: ${namespace}/g" "${temp_rbac}"
    sed -i "s/namespace: kube-system/namespace: ${namespace}/g" "${temp_snap_controller}"
    sed -i -E "s/(image: registry\.k8s\.io\/sig-storage\/snapshot-controller:).*$/\1$SNAPSHOT_VERSION/g" "${temp_snap_controller}"

    if [ "${operation}" == "create" ]; then
        # Argument to add/update
        ARGUMENT="--enable-volume-group-snapshots=true"
        # Check if the argument is already present and set to false
        if grep -q -E "^\s+-\s+--enable-volume-group-snapshots=false" "${temp_snap_controller}"; then
            sed -i -E "s/^\s+-\s+--enable-volume-group-snapshots=false$/      - $ARGUMENT/" "${temp_snap_controller}"
            # Check if the argument is already present and set to true
        elif grep -q -E "^\s+-\s+--enable-volume-group-snapshots=true" "${temp_snap_controller}"; then
            echo "Argument already present and matching."
        else
            # Add the argument if it's not present
            sed -i -E "/^(\s+)args:/a\           \ - $ARGUMENT" "${temp_snap_controller}"
        fi
    fi

    kubectl_retry "${operation}" -f "${VOLUME_GROUP_SNAPSHOTCLASS}"
    kubectl_retry "${operation}" -f "${VOLUME_GROUP_SNAPSHOT_CONTENT}"
    kubectl_retry "${operation}" -f "${VOLUME_GROUP_SNAPSHOT}"
    kubectl_retry "${operation}" -f "${temp_rbac}"
    kubectl_retry "${operation}" -f "${temp_snap_controller}" -n "${namespace}"
    kubectl_retry "${operation}" -f "${SNAPSHOTCLASS}"
    kubectl_retry "${operation}" -f "${VOLUME_SNAPSHOT_CONTENT}"
    kubectl_retry "${operation}" -f "${VOLUME_SNAPSHOT}"
}

case "${1:-}" in
install)
    install_snapshot_controller "$2"
    ;;
cleanup)
    cleanup_snapshot_controller "$2"
    ;;
*)
    echo "usage:" >&2
    echo "  $0 install" >&2
    echo "  $0 cleanup" >&2
    ;;
esac
