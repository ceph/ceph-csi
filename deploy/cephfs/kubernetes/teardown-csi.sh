#!/bin/bash

objects=(cephfsplugin csi-provisioner csi-attacher cephfs-storage-class)

for obj in ${objects[@]}; do
	kubectl delete -f "./$obj.yaml"
done
