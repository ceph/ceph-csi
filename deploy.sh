#!/bin/bash

push_helm_charts() {
	PACKAGE=$1
	VERSION=${ENV_CSI_IMAGE_VERSION//v/} # Set version (without v prefix)

	# update information in Chart.yaml if the branch is not master
	if [ "$TRAVIS_BRANCH" != "master" ]; then
		# Replace appVersion: canary and version: *-canary with the actual version
		sed -i "s/\(\s.*canary\)/ $VERSION/" "charts/ceph-csi-$PACKAGE/Chart.yaml"

		if [[ "$VERSION" == *"canary"* ]]; then
			# Replace master with the version branch
			sed -i "s/master/$TRAVIS_BRANCH/" "charts/ceph-csi-$PACKAGE/Chart.yaml"
		else
			# This is not a canary release, replace master with the tagged branch
			sed -i "s/master/v$VERSION/" "charts/ceph-csi-$PACKAGE/Chart.yaml"

		fi
	fi

	mkdir -p tmp/csi-charts/docs/"$PACKAGE"
	pushd tmp/csi-charts/docs/"$PACKAGE" >/dev/null
	helm init --client-only
	helm package ../../../../charts/ceph-csi-"$PACKAGE"
	popd >/dev/null

	pushd tmp/csi-charts/docs >/dev/null
	helm repo index .
	git add --all :/ && git commit -m "Update for helm charts $PACKAGE-$VERSION"
	git push https://"$GITHUB_TOKEN"@github.com/ceph/csi-charts
	popd >/dev/null

}

if [ "${TRAVIS_BRANCH}" == 'master' ]; then
	export ENV_CSI_IMAGE_VERSION='canary'
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
