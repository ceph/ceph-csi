#!/bin/bash

deployment_base="${1}"

if [[ -z $deployment_base ]]; then
	deployment_base="../../deploy/cephfs/kubernetes"
fi

cd "$deployment_base" || exit 1

objects=(csi-cephfsplugin-provisioner csi-cephfsplugin csi-config-map csi-provisioner-rbac csi-nodeplugin-rbac csidriver)

for obj in "${objects[@]}"; do
	kubectl delete -f "./$obj.yaml"
done
