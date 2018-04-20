#!/bin/bash

objects=(cephfs-storage-class cephfsplugin csi-attacher csi-provisioner)

for obj in ${objects[@]}; do
	kubectl create -f "./$obj.yaml"
done
