#!/bin/bash -e

#Based on ideas from https://github.com/rook/rook/blob/master/tests/scripts/helm.sh

TEMP="/tmp/cephcsi-helm-test"

HELM="helm"
HELM_VERSION=${HELM_VERSION:-"v2.16.5"}
arch="${ARCH:-}"
CEPHFS_CHART_NAME="ceph-csi-cephfs"
RBD_CHART_NAME="ceph-csi-rbd"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEPLOY_TIMEOUT=600

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
        wget "https://storage.googleapis.com/kubernetes-helm/helm-${HELM_VERSION}-${dist}-${arch}.tar.gz" -O "${TEMP}/helm.tar.gz"
        tar -C "${TEMP}" -zxvf "${TEMP}/helm.tar.gz"
    fi

    # set up RBAC for helm
    kubectl --namespace kube-system create sa tiller
    kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller

    # Init helm
    "${HELM}" init --service-account tiller --output yaml |
        sed 's@apiVersion: extensions/v1beta1@apiVersion: apps/v1@' |
        sed 's@strategy: {}@selector: {"matchLabels": {"app": "helm", "name": "tiller"}}@' | kubectl apply -f -

    kubectl -n kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'

    sleep 5

    helm_ready=$(kubectl get pods -l app=helm -n kube-system -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
    INC=0
    until [[ "${helm_ready}" == "true" || $INC -gt 20 ]]; do
        sleep 10
        ((++INC))
        helm_ready=$(kubectl get pods -l app=helm -n kube-system -o jsonpath='{.items[0].status.containerStatuses[0].ready}')
        echo "helm pod status: ${helm_ready}"
    done

    if [ "${helm_ready}" != "true" ]; then
        echo "Helm init not successful"
        kubectl get pods -l app=helm -n kube-system
        kubectl logs -lapp=helm --all-containers=true -nkube-system
        exit 1
    fi

    echo "Helm init successful"
}

install_cephcsi_helm_charts() {
    NAMESPACE=$1
    if [ -z "$NAMESPACE" ]; then
        NAMESPACE="default"
    fi
    # install ceph-csi-cephfs and ceph-csi-rbd charts
    "${HELM}" install "${SCRIPT_DIR}"/../charts/ceph-csi-cephfs --name ${CEPHFS_CHART_NAME} --namespace ${NAMESPACE} --set provisioner.fullnameOverride=csi-cephfsplugin-provisioner --set nodeplugin.fullnameOverride=csi-cephfsplugin --set configMapName=ceph-csi-config --set provisioner.podSecurityPolicy.enabled=true --set nodeplugin.podSecurityPolicy.enabled=true

    check_deployment_status app=ceph-csi-cephfs ${NAMESPACE}
    check_daemonset_status app=ceph-csi-cephfs ${NAMESPACE}

    # deleting configmap as a workaround to avoid configmap already present
    # issue when installing ceph-csi-rbd
    kubectl delete cm ceph-csi-config --namespace ${NAMESPACE}
    "${HELM}" install "${SCRIPT_DIR}"/../charts/ceph-csi-rbd --name ${RBD_CHART_NAME} --namespace ${NAMESPACE} --set provisioner.fullnameOverride=csi-rbdplugin-provisioner --set nodeplugin.fullnameOverride=csi-rbdplugin --set configMapName=ceph-csi-config --set provisioner.podSecurityPolicy.enabled=true --set nodeplugin.podSecurityPolicy.enabled=true

    check_deployment_status app=ceph-csi-rbd ${NAMESPACE}
    check_daemonset_status app=ceph-csi-rbd ${NAMESPACE}

}

cleanup_cephcsi_helm_charts() {
    "${HELM}" del --purge ${CEPHFS_CHART_NAME}
    "${HELM}" del --purge ${RBD_CHART_NAME}
}

helm_reset() {
    "${HELM}" reset
    # shellcheck disable=SC2021
    rm -rf "${TEMP}"
    kubectl --namespace kube-system delete sa tiller
    kubectl delete clusterrolebinding tiller
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
    cleanup_cephcsi_helm_charts
    ;;
*)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 clean" >&2
    echo "  $0 install-cephcsi" >&2
    echo "  $0 cleanup-cephcsi" >&2
    ;;
esac
