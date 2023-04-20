#!/bin/bash -E

#Based on ideas from https://github.com/rook/rook/blob/master/tests/scripts/helm.sh

TEMP="/tmp/cephcsi-helm-test"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=build.env
[ ! -e "${SCRIPT_DIR}"/../build.env ] || source "${SCRIPT_DIR}"/../build.env
# shellcheck disable=SC1091
[ ! -e "${SCRIPT_DIR}"/utils.sh ] || source "${SCRIPT_DIR}"/utils.sh

HELM="helm"
HELM_VERSION=${HELM_VERSION:-"latest"}
arch="${ARCH:-}"
CEPHFS_CHART_NAME="ceph-csi-cephfs"
RBD_CHART_NAME="ceph-csi-rbd"
DEPLOY_TIMEOUT=600
DEPLOY_SC=0
DEPLOY_SECRET=0

# ceph-csi specific variables
NODE_LABEL_REGION="test.failure-domain/region"
NODE_LABEL_ZONE="test.failure-domain/zone"
REGION_VALUE="testregion"
ZONE_VALUE="testzone"

example() {
    echo "examples:" >&2
    echo "To install cephcsi helm charts in a namespace, use one of the following approaches:" >&2
    echo " " >&2
    echo "1) ./scripts/install-helm install-cephcsi" >&2
    echo "2) ./scripts/install-helm install-cephcsi <NAMESPACE>" >&2
    echo "3) ./scripts/install-helm install-cephcsi --namespace <NAMESPACE>" >&2
    echo " " >&2
    echo "To deploy storageclass or secret (both optional), use --deploy-sc or --deploy-secret" >&2
    echo " " >&2
    echo "1) ./scripts/install-helm install-cephcsi --namespace <NAMESPACE> --deploy-sc" >&2
    echo "2) ./scripts/install-helm install-cephcsi --namespace <NAMESPACE> --deploy-sc --deploy-secret" >&2
    echo " " >&2
    echo "Note: Namespace is an optional parameter, which if not provided, defaults to 'default'" >&2
    echo " " >&2
}

usage() {
    echo "usage:" >&2
    echo "  ./scripts/install-helm up" >&2
    echo "  ./scripts/install-helm clean" >&2
    echo "  ./scripts/install-helm install-cephcsi --namespace <NAMESPACE>" >&2
    echo "  ./scripts/install-helm cleanup-cephcsi --namespace <NAMESPACE>" >&2
    echo " " >&2
}

check_deployment_status() {
    LABEL=$1
    NAMESPACE=$2
    echo "Checking Deployment status for label $LABEL in Namespace $NAMESPACE"
    for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 5)); do
        total_replicas=$(kubectl_retry get deployment -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.replicas}')

        ready_replicas=$(kubectl_retry get deployment -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.readyReplicas}')
        if [ "$total_replicas" != "$ready_replicas" ]; then
            echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
            kubectl_retry get deployment -l "$LABEL" -n "$NAMESPACE"
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

check_daemonset_status() {
    LABEL=$1
    NAMESPACE=$2
    echo "Checking Daemonset status for label $LABEL in Namespace $NAMESPACE"
    for ((retry = 0; retry <= DEPLOY_TIMEOUT; retry = retry + 5)); do
        total_replicas=$(kubectl_retry get daemonset -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.numberAvailable}')

        ready_replicas=$(kubectl_retry get daemonset -l "$LABEL" -n "$NAMESPACE" -o jsonpath='{.items[0].status.numberReady}')
        if [ "$total_replicas" != "$ready_replicas" ]; then
            echo "Total replicas $total_replicas is not equal to ready count $ready_replicas"
            kubectl_retry get daemonset -l "$LABEL" -n "$NAMESPACE"
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

fetch_template_values() {
    TOOLBOX_POD=$(kubectl_retry -n rook-ceph get pods -l app=rook-ceph-tools -o=jsonpath='{.items[0].metadata.name}')
    # fetch fsid to populate the clusterID in storageclass
    FS_ID=$(kubectl_retry -n rook-ceph exec "${TOOLBOX_POD}" -- ceph fsid)
    # fetch the admin key corresponding to the adminID
    ADMIN_KEY=$(kubectl_retry -n rook-ceph exec "${TOOLBOX_POD}" -- ceph auth get-key client.admin)
}

install() {
    if ! helm_loc="$(type -p "helm")" || [[ -z ${helm_loc} ]]; then
        # Download and unpack helm
        local dist
        dist="$(uname -s)"
        mkdir -p ${TEMP}
        # shellcheck disable=SC2021
        dist=$(echo "${dist}" | tr "[A-Z]" "[a-z]")
        wget "https://get.helm.sh/helm-${HELM_VERSION}-${dist}-${arch}.tar.gz" -O "${TEMP}/helm.tar.gz" || exit 1
        tar -C "${TEMP}" -zxvf "${TEMP}/helm.tar.gz"
    fi
    echo "Helm install successful"
}

install_cephcsi_helm_charts() {
    NAMESPACE=$1
    if [ -z "$NAMESPACE" ]; then
        NAMESPACE="default"
    fi

    kubectl_retry create namespace "${NAMESPACE}"

    # label the nodes uniformly for domain information
    for node in $(kubectl_retry get node -o jsonpath='{.items[*].metadata.name}'); do
        kubectl_retry label node/"${node}" ${NODE_LABEL_REGION}=${REGION_VALUE}
        kubectl_retry label node/"${node}" ${NODE_LABEL_ZONE}=${ZONE_VALUE}
    done

    # deploy storageclass if DEPLOY_SC flag is set
    if [ "${DEPLOY_SC}" -eq 1 ]; then
        fetch_template_values
        SET_SC_TEMPLATE_VALUES="--set storageClass.create=true --set storageClass.clusterID=${FS_ID}"
    fi
    # deploy secret if DEPLOY_SECRET flag is set
    if [ "${DEPLOY_SECRET}" -eq 1 ]; then
        fetch_template_values
        RBD_SECRET_TEMPLATE_VALUES="--set secret.create=true --set secret.userID=admin --set secret.userKey=${ADMIN_KEY}"
        CEPHFS_SECRET_TEMPLATE_VALUES="--set secret.create=true --set secret.adminID=admin --set secret.adminKey=${ADMIN_KEY}"
    fi
    # install ceph-csi-cephfs and ceph-csi-rbd charts
    # shellcheck disable=SC2086
    "${HELM}" install --namespace ${NAMESPACE} --set provisioner.fullnameOverride=csi-cephfsplugin-provisioner --set nodeplugin.fullnameOverride=csi-cephfsplugin --set configMapName=ceph-csi-config --set provisioner.replicaCount=1 --set-json='commonLabels={"app.kubernetes.io/name": "ceph-csi-cephfs", "app.kubernetes.io/managed-by": "helm"}' ${SET_SC_TEMPLATE_VALUES} ${CEPHFS_SECRET_TEMPLATE_VALUES} ${CEPHFS_CHART_NAME} "${SCRIPT_DIR}"/../charts/ceph-csi-cephfs
    check_deployment_status app=ceph-csi-cephfs "${NAMESPACE}"
    check_daemonset_status app=ceph-csi-cephfs "${NAMESPACE}"

    # deleting configmaps as a workaround to avoid configmap already present
    # issue when installing ceph-csi-rbd
    kubectl_retry delete cm ceph-csi-config --namespace "${NAMESPACE}"
    kubectl_retry delete cm ceph-config --namespace "${NAMESPACE}"

    # shellcheck disable=SC2086
    "${HELM}" install --namespace ${NAMESPACE} --set provisioner.fullnameOverride=csi-rbdplugin-provisioner --set nodeplugin.fullnameOverride=csi-rbdplugin --set configMapName=ceph-csi-config --set provisioner.replicaCount=1 --set-json='commonLabels={"app.kubernetes.io/name": "ceph-csi-rbd", "app.kubernetes.io/managed-by": "helm"}' ${SET_SC_TEMPLATE_VALUES} ${RBD_SECRET_TEMPLATE_VALUES} ${RBD_CHART_NAME} "${SCRIPT_DIR}"/../charts/ceph-csi-rbd --set topology.enabled=true --set topology.domainLabels="{${NODE_LABEL_REGION},${NODE_LABEL_ZONE}}" --set provisioner.maxSnapshotsOnImage=3 --set provisioner.minSnapshotsOnImage=2

    check_deployment_status app=ceph-csi-rbd "${NAMESPACE}"
    check_daemonset_status app=ceph-csi-rbd "${NAMESPACE}"

}

cleanup_cephcsi_helm_charts() {
    # remove set labels
    for node in $(kubectl_retry get node --no-headers | cut -f 1 -d ' '); do
        kubectl_retry label node/"$node" test.failure-domain/region-
        kubectl_retry label node/"$node" test.failure-domain/zone-
    done
    # TODO/LATER we could remove the CSI labels that would have been set as well
    NAMESPACE=$1
    if [ -z "$NAMESPACE" ]; then
        NAMESPACE="default"
    fi
    "${HELM}" uninstall ${CEPHFS_CHART_NAME} --namespace "${NAMESPACE}"
    "${HELM}" uninstall ${RBD_CHART_NAME} --namespace "${NAMESPACE}"
    kubectl_retry delete namespace "${NAMESPACE}"
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

if [ "$#" -le 2 ]
then
    ACTION=$1
    NAMESPACE=$2
    SKIP_PARSE="true"
fi

if [ ${#SKIP_PARSE} -eq 0 ]; then
    while [ "$1" != "" ]
    do
        case $1 in
        up)
            shift
            ACTION="up"
            ;;
        clean)
            shift
            ACTION="clean"
            ;;
        install-cephcsi)
            shift
            ACTION="install-cephcsi"
            ;;
        cleanup-cephcsi)
            shift
            ACTION="cleanup-cephcsi"
            ;;
        --namespace)
            shift
            NAMESPACE=$1
            # validate if namespace is not empty
            if [ ${#NAMESPACE} -eq 0 ]; then
                echo "Provided namespace is empty: ${NAMESPACE}" >&2
                usage
                example
                exit 1
            fi
            shift
            ;;
        --deploy-sc)
            shift
            DEPLOY_SC=1
            ;;
        --deploy-secret)
            shift
            DEPLOY_SECRET=1
            ;;
        *)
            echo "illegal option $1"
            echo "$#"
            usage
            example
            exit 1
            ;;
        esac
    done
fi

case "${ACTION}" in
up)
    install
    ;;
clean)
    helm_reset
    ;;
install-cephcsi)
    install_cephcsi_helm_charts "${NAMESPACE}"
    ;;
cleanup-cephcsi)
    cleanup_cephcsi_helm_charts "${NAMESPACE}"
    ;;
*)
    usage
    example
    exit 1
    ;;
esac
