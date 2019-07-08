#!/bin/bash

push_helm_charts() {
	PACKAGE=$1
	CHANGED=0
	VERSION=$(grep 'version:' deploy/"$PACKAGE"/helm/Chart.yaml | awk '{print $2}')

	if [ ! -f "tmp/csi-charts/docs/$PACKAGE/v$VERSION/ceph-csi-$PACKAGE-$VERSION.tgz" ]; then
		CHANGED=1
		ln -s helm deploy/"$PACKAGE"/ceph-csi-"$PACKAGE"
		mkdir -p tmp/csi-charts/docs/"$PACKAGE/v$VERSION"
		pushd tmp/csi-charts/docs/"$PACKAGE" >/dev/null
		helm init --client-only
		helm package ../../../../deploy/"$PACKAGE"/ceph-csi-"$PACKAGE"
		popd >/dev/null
	fi

	if [ $CHANGED -eq 1 ]; then
		pushd tmp/csi-charts/docs >/dev/null
		helm repo index .
		git add --all :/ && git commit -m "Update repo"
		git push https://"$GITHUB_TOKEN"@github.com/ceph/csi-charts
		popd >/dev/null
	fi
}

if [ "${TRAVIS_BRANCH}" == 'release-v1.1.0' ]; then
	export ENV_CSI_IMAGE_VERSION='v1.1.0'
else
	echo "!!! Branch ${TRAVIS_BRANCH} is not a deployable branch; exiting"
	exit 0 # Exiting 0 so that this isn't marked as failing
fi

if [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
	"${CONTAINER_CMD:-docker}" login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io
	make push-image-cephcsi

	set -xe

	mkdir -p tmp
	pushd tmp >/dev/null

	curl https://raw.githubusercontent.com/helm/helm/master/scripts/get >get_helm.sh
	chmod 700 get_helm.sh
	./get_helm.sh

	git clone https://github.com/ceph/csi-charts

	mkdir -p csi-charts/docs
	popd >/dev/null

	push_helm_charts rbd
	push_helm_charts cephfs
fi
