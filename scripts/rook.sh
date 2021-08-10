#!/bin/bash -E

ROOK_VERSION=${ROOK_VERSION:-"v1.6.2"}
ROOK_DEPLOY_TIMEOUT=${ROOK_DEPLOY_TIMEOUT:-300}
ROOK_URL="https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/cluster/examples/kubernetes/ceph"
ROOK_BLOCK_POOL_NAME=${ROOK_BLOCK_POOL_NAME:-"newrbdpool"}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck disable=SC1091
[ ! -e "${SCRIPT_DIR}"/utils.sh ] || source "${SCRIPT_DIR}"/utils.sh

trap log_errors ERR

# log_errors is called on exit (see 'trap' above) and tries to provide
# sufficient information to debug deployment problems
function log_errors() {
	# enable verbose execution
	set -x
	kubectl get nodes
	kubectl -n rook-ceph get events
	kubectl -n rook-ceph describe pods
	kubectl -n rook-ceph logs -l app=rook-ceph-operator
	kubectl -n rook-ceph get CephClusters -oyaml
	kubectl -n rook-ceph get CephFilesystems -oyaml
	kubectl -n rook-ceph get CephBlockPools -oyaml

	# this function should not return, a fatal error was caught!
	exit 1
}

rook_version() {
	echo "${ROOK_VERSION#v}" | cut -d'.' -f"${1}"
}

function deploy_rook() {
        kubectl_retry create -f "${ROOK_URL}/common.yaml"

        # If rook version is > 1.5 , we will apply CRDs.
        ROOK_MAJOR=$(rook_version 1)
        ROOK_MINOR=$(rook_version 2)
        if  [ "${ROOK_MAJOR}" -eq 1 ] && [ "${ROOK_MINOR}" -ge 5 ];
	  then
	      kubectl_retry create -f "${ROOK_URL}/crds.yaml"
	  fi
        kubectl_retry create -f "${ROOK_URL}/operator.yaml"
        # Override the ceph version which rook installs by default.
        if  [ -z "${ROOK_CEPH_CLUSTER_IMAGE}" ]
        then
            kubectl_retry create -f "${ROOK_URL}/cluster-test.yaml"
        else
            ROOK_CEPH_CLUSTER_VERSION_IMAGE_PATH="image: ${ROOK_CEPH_CLUSTER_IMAGE}"
            TEMP_DIR="$(mktemp -d)"
            curl -o "${TEMP_DIR}"/cluster-test.yaml "${ROOK_URL}/cluster-test.yaml"
            sed -i "s|image.*|${ROOK_CEPH_CLUSTER_VERSION_IMAGE_PATH}|g" "${TEMP_DIR}"/cluster-test.yaml
            sed -i "s/config: |/config: |\n    \[mon\]\n    mon_warn_on_insecure_global_id_reclaim_allowed = false/g" "${TEMP_DIR}"/cluster-test.yaml
            sed -i "s/healthCheck:/healthCheck:\n    livenessProbe:\n      mon:\n        disabled: true\n      mgr:\n        disabled: true\n      mds:\n        disabled: true/g" "${TEMP_DIR}"/cluster-test.yaml
			cat  "${TEMP_DIR}"/cluster-test.yaml
            kubectl_retry create -f "${TEMP_DIR}/cluster-test.yaml"
            rm -rf "${TEMP_DIR}"
        fi

        kubectl_retry create -f "${ROOK_URL}/toolbox.yaml"
        kubectl_retry create -f "${ROOK_URL}/filesystem-test.yaml"
        kubectl_retry create -f "${ROOK_URL}/pool-test.yaml"

        # Check if CephCluster is empty
        if ! kubectl_retry -n rook-ceph get cephclusters -oyaml | grep 'items: \[\]' &>/dev/null; then
            check_ceph_cluster_health
        fi

        # Check if CephFileSystem is empty
        if ! kubectl_retry -n rook-ceph get cephfilesystems -oyaml | grep 'items: \[\]' &>/dev/null; then
            check_mds_stat
        fi

        # Check if CephBlockPool is empty
        if ! kubectl_retry -n rook-ceph get cephblockpools -oyaml | grep 'items: \[\]' &>/dev/null; then
            check_rbd_stat ""
        fi
}

function teardown_rook() {
	kubectl delete -f "${ROOK_URL}/pool-test.yaml"
	kubectl delete -f "${ROOK_URL}/filesystem-test.yaml"
	kubectl delete -f "${ROOK_URL}/toolbox.yaml"
	kubectl delete -f "${ROOK_URL}/cluster-test.yaml"
	kubectl delete -f "${ROOK_URL}/operator.yaml"
	ROOK_MAJOR=$(rook_version 1)
      ROOK_MINOR=$(rook_version 2)
      if  [ "${ROOK_MAJOR}" -eq 1 ] && [ "${ROOK_MINOR}" -ge 5 ];
	then
	      kubectl delete -f "${ROOK_URL}/crds.yaml"
	fi
	kubectl delete -f "${ROOK_URL}/common.yaml"
}

function create_block_pool() {
	curl -o newpool.yaml "${ROOK_URL}/pool-test.yaml"
	sed -i "s/replicapool/$ROOK_BLOCK_POOL_NAME/g" newpool.yaml
	kubectl_retry create -f "./newpool.yaml"
	rm -f "./newpool.yaml"

	check_rbd_stat "$ROOK_BLOCK_POOL_NAME"
}

function delete_block_pool() {
	curl -o newpool.yaml "${ROOK_URL}/pool-test.yaml"
	sed -i "s/replicapool/$ROOK_BLOCK_POOL_NAME/g" newpool.yaml
	kubectl delete -f "./newpool.yaml"
	rm -f "./newpool.yaml"
}

function check_ceph_cluster_health() {
	for ((retry = 0; retry <= ROOK_DEPLOY_TIMEOUT; retry = retry + 5)); do
		echo "Wait for rook deploy... ${retry}s" && sleep 5

		CEPH_STATE=$(kubectl_retry -n rook-ceph get cephclusters -o jsonpath='{.items[0].status.state}')
		CEPH_HEALTH=$(kubectl_retry -n rook-ceph get cephclusters -o jsonpath='{.items[0].status.ceph.health}')
		echo "Checking CEPH cluster state: [$CEPH_STATE]"
		if [ "$CEPH_STATE" = "Created" ]; then
			if [ "$CEPH_HEALTH" = "HEALTH_OK" ]; then
				echo "Creating CEPH cluster is done. [$CEPH_HEALTH]"
				break
			fi
		fi
	done

	if [ "$retry" -gt "$ROOK_DEPLOY_TIMEOUT" ]; then
		echo "[Timeout] CEPH cluster not in a healthy state (timeout)"
		return 1
	fi
	echo ""
}

function check_mds_stat() {
	for ((retry = 0; retry <= ROOK_DEPLOY_TIMEOUT; retry = retry + 5)); do
		FS_NAME=$(kubectl_retry -n rook-ceph get cephfilesystems.ceph.rook.io -ojsonpath='{.items[0].metadata.name}')
		echo "Checking MDS ($FS_NAME) stats... ${retry}s" && sleep 5

		ACTIVE_COUNT=$(kubectl_retry -n rook-ceph get cephfilesystems myfs -ojsonpath='{.spec.metadataServer.activeCount}')

		ACTIVE_COUNT_NUM=$((ACTIVE_COUNT + 0))
		echo "MDS ($FS_NAME) active_count: [$ACTIVE_COUNT_NUM]"
		if ((ACTIVE_COUNT_NUM < 1)); then
			continue
		else
			if kubectl_retry -n rook-ceph get pod -l rook_file_system=myfs | grep Running &>/dev/null; then
				echo "Filesystem ($FS_NAME) is successfully created..."
				break
			fi
		fi
	done

	if [ "$retry" -gt "$ROOK_DEPLOY_TIMEOUT" ]; then
		echo "[Timeout] Failed to get ceph filesystem pods"
		return 1
	fi
	echo ""
}

function check_rbd_stat() {
	for ((retry = 0; retry <= ROOK_DEPLOY_TIMEOUT; retry = retry + 5)); do
		if [ -z "$1" ]; then
			RBD_POOL_NAME=$(kubectl_retry -n rook-ceph get cephblockpools -ojsonpath='{.items[0].metadata.name}')
		else
			RBD_POOL_NAME=$1
		fi
		echo "Checking RBD ($RBD_POOL_NAME) stats... ${retry}s" && sleep 5

		TOOLBOX_POD=$(kubectl_retry -n rook-ceph get pods -l app=rook-ceph-tools -o jsonpath='{.items[0].metadata.name}')
		TOOLBOX_POD_STATUS=$(kubectl_retry -n rook-ceph get pod "$TOOLBOX_POD" -ojsonpath='{.status.phase}')
		[[ "$TOOLBOX_POD_STATUS" != "Running" ]] && \
			{ echo "Toolbox POD ($TOOLBOX_POD) status: [$TOOLBOX_POD_STATUS]"; continue; }

		if kubectl_retry exec -n rook-ceph "$TOOLBOX_POD" -it -- rbd pool stats "$RBD_POOL_NAME" &>/dev/null; then
			echo "RBD ($RBD_POOL_NAME) is successfully created..."
			break
		fi
	done

	if [ "$retry" -gt "$ROOK_DEPLOY_TIMEOUT" ]; then
		echo "[Timeout] Failed to get RBD pool stats"
		return 1
	fi
	echo ""
}

case "${1:-}" in
deploy)
	deploy_rook
	;;
teardown)
	teardown_rook
	;;
create-block-pool)
	create_block_pool
	;;
delete-block-pool)
	delete_block_pool
	;;
*)
	echo " $0 [command]
Available Commands:
  deploy             Deploy a rook
  teardown           Teardown a rook
  create-block-pool  Create a rook block pool
  delete-block-pool  Delete a rook block pool
" >&2
	;;
esac
