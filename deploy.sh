#!/bin/bash

push_helm_charts() {
	PACKAGE=$1
	CHANGED=0
	VERSION=${ENV_CSI_IMAGE_VERSION//v}  # Set version (without v prefix)

  # Always run when version is canary, when versioned only when the package doesn't exist yet
	if [ ! -f "tmp/csi-charts/docs/$PACKAGE/ceph-csi-$PACKAGE-$VERSION.tgz" ] && [ -z "$VERSION" ]; then
		CHANGED=1

    # When version defined it is a release, not a canary build
    if [ -z "$VERSION" ]; then
      # Replace appVersion: canary and version: *-canary with the actual version
      sed -i "s/\(\s.*canary\)/$VERSION/" "charts/ceph-csi-$PACKAGE/Chart.yaml"

      # Replace master with the version branch
      sed -i "s/tree\/master/tree\/release-v$VERSION/" "charts/ceph-csi-$PACKAGE/Chart.yaml"
    fi

		ln -s helm charts/ceph-csi-"$PACKAGE"
		mkdir -p tmp/csi-charts/docs/"$PACKAGE"
		pushd tmp/csi-charts/docs/"$PACKAGE" >/dev/null
		helm init --client-only
		helm package ../../../../charts/ceph-csi-"$PACKAGE"
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

if [ "${TRAVIS_BRANCH}" == 'csi-v0.3' ]; then
	export ENV_RBD_IMAGE_VERSION='v0.3-canary'
	export ENV_CEPHFS_IMAGE_VERSION='v0.3-canary'
elif [ "${TRAVIS_BRANCH}" == 'master' ]; then
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
