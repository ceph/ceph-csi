#!/bin/bash -e

ROOK_VERSION=${ROOK_VERSION:-"v1.1.7"}
ROOK_DEPLOY_TIMEOUT=${ROOK_DEPLOY_TIMEOUT:-300}
ROOK_URL="https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/cluster/examples/kubernetes/ceph"

function deploy_rook() {
    kubectl create -f "${ROOK_URL}/common.yaml"
    kubectl create -f "${ROOK_URL}/operator.yaml"
    kubectl create -f "${ROOK_URL}/cluster-test.yaml"
    kubectl create -f "${ROOK_URL}/toolbox.yaml"
    kubectl create -f "${ROOK_URL}/filesystem-test.yaml"
    kubectl create -f "${ROOK_URL}/pool-test.yaml"

    for ((retry=0; retry<=ROOK_DEPLOY_TIMEOUT; retry=retry+5)); do
        echo "Wait for rook deploy... ${retry}s"
        sleep 5

        if kubectl get cephclusters -n rook-ceph | grep HEALTH_OK &> /dev/null; then
            break
        fi
    done
}

function teardown_rook() {
    kubectl delete -f "${ROOK_URL}/pool-test.yaml"
    kubectl delete -f "${ROOK_URL}/filesystem-test.yaml"
    kubectl delete -f "${ROOK_URL}/toolbox.yaml"
    kubectl delete -f "${ROOK_URL}/cluster-test.yaml"
    kubectl delete -f "${ROOK_URL}/operator.yaml"
    kubectl delete -f "${ROOK_URL}/common.yaml"
}

case "${1:-}" in
deploy)
    deploy_rook
    ;;
teardown)
    teardown_rook
    ;;
*)
    echo " $0 [command]
Available Commands:
  deploy         Deploy a rook
  teardown       Teardown a rook
" >&2
    ;;
esac
