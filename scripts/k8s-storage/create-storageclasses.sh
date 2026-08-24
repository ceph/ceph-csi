#!/bin/sh
#
# Create StorageClasses from a template (sc-*.yaml.in) and replace keywords
# like @@CLUSTER_ID@@.
#
# These StorageClasses can then be used by driver-*.yaml manifests in the
# k8s-e2e-external-storage CI job.
#
# Requirements:
# - kubectl in the path
# - working KUBE_CONFIG either in environment, or default config files
# - deployment done with Rook
#

# exit on error
set -e

WORKDIR=$(dirname "${0}")

TOOLBOX_POD=$(kubectl -n rook-ceph get pods --no-headers -l app=rook-ceph-tools -o=jsonpath='{.items[0].metadata.name}')
FS_ID=$(kubectl -n rook-ceph exec "${TOOLBOX_POD}" -- ceph fsid)

# NVMeoF-specific parameters (can be overridden via environment variables)
GATEWAY_ADDRESS=${GATEWAY_ADDRESS:-""}
LISTENERS=${LISTENERS:-""}
SHORT_HOSTNAME=${SHORT_HOSTNAME:-""}
# FIXME: Only pass "hostname" in the LISTENERS, no "address" and "port".
# POD_ADDRESS is needed as the nvmeof-gw is deployed in rook-ceph, and ceph-csi
# in a dedicated testing namespace. Resolving the short-hostname of the gateway
# is not possible from outside the rook-ceph namespace. The gateway is
# configured with a host-id as the short-hostname, which needs to match what is
# passed in the LISTENERS.
POD_ADDRESS=${POD_ADDRESS:-""}

# Auto-detect gateway address and listeners from rook-ceph-nvmeof service if not set
if [ -z "${GATEWAY_ADDRESS}" ]; then
	GATEWAY_ADDRESS=$(kubectl -n rook-ceph get service -l app=rook-ceph-nvmeof -o=jsonpath='{.items[0].spec.clusterIP}' 2>/dev/null || echo "")
fi

if [ -z "${SHORT_HOSTNAME}" ]; then
	SHORT_HOSTNAME=$(kubectl -n rook-ceph get service -l app=rook-ceph-nvmeof -o=jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
fi

if [ -z "${POD_ADDRESS}" ]; then
	POD_ADDRESS=$(kubectl -n rook-ceph get pod -l app=rook-ceph-nvmeof -o=jsonpath='{.items[0].status.podIP}' 2>/dev/null || echo "")
fi

if [ -z "${LISTENERS}" ] && [ -n "${POD_ADDRESS}" ] && [ -n "${SHORT_HOSTNAME}" ]; then
	# Create a simple listener config with the gateway pod IP address and hostname
	LISTENERS='[{"address": "'"${POD_ADDRESS}"'", "port": 4420, "hostname": "'"${SHORT_HOSTNAME}"'"}]'
fi

for sc in "${WORKDIR}"/sc-*.yaml.in
do
	# Start with CLUSTER_ID replacement
	SC_CONTENT=$(sed "s/@@CLUSTER_ID@@/${FS_ID}/" "${sc}")

	# For nvmeof, also replace nvmeof-specific parameters
	if echo "${sc}" | grep -q "nvmeof"; then
		if [ -z "${GATEWAY_ADDRESS}" ]; then
			echo "Warning: GATEWAY_ADDRESS not set and could not auto-detect rook-ceph-nvmeof service"
			echo "Skipping ${sc}"
			continue
		fi
		if [ -z "${LISTENERS}" ]; then
			echo "Warning: LISTENERS not set and could not auto-detect rook-ceph-nvmeof hostnames"
			echo "Skipping ${sc}"
			continue
		fi

		SC_CONTENT=$(echo "${SC_CONTENT}" | sed \
			-e "s|@@GATEWAY_ADDRESS@@|${GATEWAY_ADDRESS}|g" \
			-e "s|@@LISTENERS@@|${LISTENERS}|g")
	fi

	echo "${SC_CONTENT}" | kubectl create -f -
done
