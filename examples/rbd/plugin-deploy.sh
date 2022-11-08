#!/bin/bash

deployment_base="${1}"
shift
kms_base="${1}"

if [[ -z "${deployment_base}" ]]; then
	deployment_base="../../deploy/rbd/kubernetes"
fi

pushd "${deployment_base}" >/dev/null || exit 1

objects=(csi-provisioner-rbac csi-nodeplugin-rbac csi-config-map csi-rbdplugin-provisioner csi-rbdplugin csidriver)

for obj in "${objects[@]}"; do
	kubectl create -f "./${obj}.yaml"
done

popd >/dev/null || exit 1

if [[ -z "${kms_base}" ]]; then
	kms_base="../kms/vault"
fi

pushd "${kms_base}" >/dev/null || exit 1

objects=(vault csi-vaulttokenreview-rbac kms-config)

for obj in "${objects[@]}"; do
	kubectl create -f "./${obj}.yaml"
done

popd >/dev/null || exit 1
