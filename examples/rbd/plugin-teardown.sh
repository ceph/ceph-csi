#!/bin/bash

deployment_base="${1}"

if [[ -z $deployment_base ]]; then
	deployment_base="../../deploy/rbd/kubernetes"
fi

cd "$deployment_base" || exit 1

objects=(csi-rbdplugin-provisioner csi-rbdplugin csi-config-map csi-provisioner-rbac csi-nodeplugin-rbac)

for obj in "${objects[@]}"; do
	kubectl delete -f "./$obj.yaml"
done
