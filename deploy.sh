#!/bin/bash

if [ "${TRAVIS_BRANCH}" == 'master' ]; then
	export RBD_IMAGE_VERSION='v0.3.0';
	export CEPHFS_IMAGE_VERSION='v0.3.0';
elif [ "${TRAVIS_BRANCH}" == 'csi-v1.0' ]; then
	export RBD_IMAGE_VERSION='v1.0.0';
	export CEPHFS_IMAGE_VERSION='v1.0.0';
else
	echo "!!! Branch ${TRAVIS_BRANCH} is not a deployable branch; exiting";
	exit 0; # Exiting 0 so that this isn't marked as failing
fi;

if [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
	docker login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io
	make push-image-rbdplugin push-image-cephfsplugin

	set -e
	
	mkdir -p tmp
	pushd tmp > /dev/null
	
	curl https://raw.githubusercontent.com/helm/helm/master/scripts/get > get_helm.sh
	chmod 700 get_helm.sh
	./get_helm.sh
	
	git clone https://github.com/ceph/csi-charts
	
	mkdir -p csi-charts/docs
        popd > /dev/null

	CHANGED=0
	VERSION=$(cat deploy/rbd/helm/Chart.yaml | awk '{if(/^version:/){print $2}}')
	
	if [ ! -f "tmp/csi-charts/docs/rbd/ceph-csi-rbd-$VERSION.tgz" ]; then
	    CHANGED=1
	    ln -s deploy/rbd/helm/ deploy/rbd/ceph-csi-rbd
	    mkdir -p tmp/csi-charts/docs/rbd
	    pushd tmp/csi-charts/docs/rbd > /dev/null
	    helm package ../../../../deploy/rbd/ceph-csi-rbd
	    popd > /dev/null
	fi
	
	if [ $CHANGED -eq 1 ]; then
	    pushd tmp/csi-charts/docs > /dev/null
	    helm repo index .
	    git add --all :/ && git commit -m "Update repo"
	    git push https://"$GITHUB_TOKEN"@github.com/ceph/csi-charts
	    popd > /dev/null
	fi

fi;

