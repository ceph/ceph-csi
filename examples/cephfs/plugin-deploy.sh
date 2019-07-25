#!/bin/bash

deployment_base="${1}"

if [[ -z $deployment_base ]]; then
	deployment_base="../../deploy/cephfs/kubernetes/v1.14+"
fi

cd "$deployment_base" || exit 1

objects=(csi-provisioner-rbac csi-nodeplugin-rbac csi-config-map csi-cephfsplugin-provisioner csi-cephfsplugin)

for obj in "${objects[@]}"; do
	kubectl create -f "./$obj.yaml"
done
