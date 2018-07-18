#!/bin/bash

deployment_base="${1}"

if [[ -z $deployment_base ]]; then
	deployment_base="../../deploy/rbd/kubernetes"
fi

cd "$deployment_base" || exit 1

objects=(csi-attacher-rbac csi-provisioner-rbac csi-nodeplugin-rbac csi-rbdplugin-attacher csi-rbdplugin-provisioner csi-rbdplugin)

for obj in ${objects[@]}; do
	kubectl create -f "./$obj.yaml"
done
