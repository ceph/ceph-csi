#!/bin/bash -e

#Based on ideas from https://github.com/rook/rook/blob/master/tests/scripts/helm.sh

TEMP="/tmp/cephcsi-helm-test"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=build.env
[ ! -e "${SCRIPT_DIR}"/../build.env ] || source "${SCRIPT_DIR}"/../build.env

HELM="helm"
HELM_VERSION=${HELM_VERSION:-"latest"}
arch="${ARCH:-}"
CEPHFS_CHART_NAME="ceph-csi-cephfs"
RBD_CHART_NAME="ceph-csi-rbd"
DEPLOY_TIMEOUT=600

# ceph-csi specific variables
NODE_LABEL_REGION="test.failure-domain/region"
NODE_LABEL_ZONE="test.failure-domain/zone"
REGION_VALUE="testregion"
ZONE_VALUE="testzone"

function check_deployment_status() {
    LABEL=$1
    NAMESPACE=$2
    echo "Checking Deployment status for label $LABEL in Namespace $NAMESPACE"
    for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 5)); do
        total_replicas=$(kubectl get deployment -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.replicas}')

        ready_replicas=$(kubectl get deployment -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.readyReplicas}')
        if [ "$total_replicas" != "$ready_replicas" ]; then
            echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
            kubectl get deployment -l "$LABEL" -n "$NAMESPACE"
            sleep 10
        else
            echo "Total replicas $total_replicas is equal to ready count $ready_replicas"
            break
        fi
    done

    if [ "$retry" -gt "$DEPLOY_TIMEOUT" ]; then
        echo "[Timeout] Failed to get deployment"
        exit 1
    fi
}

function check_daemonset_status() {
    LABEL=$1
    NAMESPACE=$2
    echo "Checking Daemonset status for label $LABEL in Namespace $NAMESPACE"
    for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 5)); do
        total_replicas=$(kubectl get daemonset -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.numberAvailable}')

        ready_replicas=$(kubectl get daemonset -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.numberReady}')
        if [ "$total_replicas" != "$ready_replicas" ]; then
            echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
            kubectl get daemonset -l "$LABEL" -n "$NAMESPACE"
            sleep 10
        else
            echo "Total replicas $total_replicas is equal to ready count $ready_replicas"
            break

        fi
    done

    if [ "$retry" -gt "$DEPLOY_TIMEOUT" ]; then
        echo "[Timeout] Failed to get daemonset"
        exit 1
    fi
}

detectArch() {
    case "$(uname -m)" in
    "x86_64" | "amd64")
        arch="amd64"
        ;;
    "aarch64")
        arch="arm64"
        ;;
    "i386")
        arch="i386"
        ;;
    *)
        echo "Couldn't translate 'uname -m' output to an available arch."
        echo "Try setting ARCH environment variable to your system arch:"
        echo "amd64, x86_64. aarch64, i386"
        exit 1
        ;;
    esac
}

install() {
    if ! helm_loc="$(type -p "helm")" || [[ -z ${helm_loc} ]]; then
        # Download and unpack helm
        local dist
        dist="$(uname -s)"
        mkdir -p ${TEMP}
        # shellcheck disable=SC2021
        dist=$(echo "${dist}" | tr "[A-Z]" "[a-z]")
        wget "https://get.helm.sh/helm-${HELM_VERSION}-${dist}-${arch}.tar.gz" -O "${TEMP}/helm.tar.gz"
        tar -C "${TEMP}" -zxvf "${TEMP}/helm.tar.gz"
    fi
    echo "Helm install successful"
}

install_cephcsi_helm_charts() {
    NAMESPACE=$1
    if [ -z "$NAMESPACE" ]; then
        NAMESPACE="default"
    fi

    # label the nodes uniformly for domain information
    for node in $(kubectl get node -o jsonpath='{.items[*].metadata.name}'); do
        kubectl label node/"${node}" ${NODE_LABEL_REGION}=${REGION_VALUE}
        kubectl label node/"${node}" ${NODE_LABEL_ZONE}=${ZONE_VALUE}
    done

    # install ceph-csi-cephfs and ceph-csi-rbd charts
    "${HELM}" install --namespace ${NAMESPACE} --set provisioner.fullnameOverride=csi-cephfsplugin-provisioner --set nodeplugin.fullnameOverride=csi-cephfsplugin --set configMapName=ceph-csi-config --set provisioner.podSecurityPolicy.enabled=true --set nodeplugin.podSecurityPolicy.enabled=true --set provisioner.replicaCount=1 ${CEPHFS_CHART_NAME} "${SCRIPT_DIR}"/../charts/ceph-csi-cephfs

    check_deployment_status app=ceph-csi-cephfs ${NAMESPACE}
    check_daemonset_status app=ceph-csi-cephfs ${NAMESPACE}

    # deleting configmap as a workaround to avoid configmap already present
    # issue when installing ceph-csi-rbd
    kubectl delete cm ceph-csi-config --namespace ${NAMESPACE}
    "${HELM}" install --namespace ${NAMESPACE} --set provisioner.fullnameOverride=csi-rbdplugin-provisioner --set nodeplugin.fullnameOverride=csi-rbdplugin --set configMapName=ceph-csi-config --set provisioner.podSecurityPolicy.enabled=true --set nodeplugin.podSecurityPolicy.enabled=true --set provisioner.replicaCount=1 ${RBD_CHART_NAME} "${SCRIPT_DIR}"/../charts/ceph-csi-rbd --set topology.enabled=true --set topology.domainLabels="{${NODE_LABEL_REGION},${NODE_LABEL_ZONE}}" --set provisioner.maxSnapshotsOnImage=3 --set provisioner.minSnapshotsOnImage=2

    check_deployment_status app=ceph-csi-rbd ${NAMESPACE}
    check_daemonset_status app=ceph-csi-rbd ${NAMESPACE}

}

cleanup_cephcsi_helm_charts() {
    # remove set labels
    for node in $(kubectl get node --no-headers | cut -f 1 -d ' '); do
        kubectl label node/"$node" test.failure-domain/region-
        kubectl label node/"$node" test.failure-domain/zone-
    done
    # TODO/LATER we could remove the CSI labels that would have been set as well
    NAMESPACE=$1
    if [ -z "$NAMESPACE" ]; then
        NAMESPACE="default"
    fi
    "${HELM}" uninstall ${CEPHFS_CHART_NAME} --namespace ${NAMESPACE}
    "${HELM}" uninstall ${RBD_CHART_NAME} --namespace ${NAMESPACE}
}

helm_reset() {
    # shellcheck disable=SC2021
    rm -rf "${TEMP}"
}

if [ -z "${arch}" ]; then
    detectArch
fi

if ! helm_loc="$(type -p "helm")" || [[ -z ${helm_loc} ]]; then
    dist="$(uname -s)"
    # shellcheck disable=SC2021
    dist=$(echo "${dist}" | tr "[A-Z]" "[a-z]")
    HELM="${TEMP}/${dist}-${arch}/helm"
fi

case "${1:-}" in
up)
    install
    ;;
clean)
    helm_reset
    ;;
install-cephcsi)
    install_cephcsi_helm_charts "$2"
    ;;
cleanup-cephcsi)
    cleanup_cephcsi_helm_charts "$2"
    ;;
*)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 clean" >&2
    echo "  $0 install-cephcsi" >&2
    echo "  $0 cleanup-cephcsi" >&2
    ;;
esac
