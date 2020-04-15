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
			sed -i "s/master/v$VERSION/" "charts/ceph-csi-$PACKAGE/templates/NOTES.txt"
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

# Build and push images. Steps as below:
# 1. get base image from original Dockerfile (FROM ceph/ceph:v14.2)
# 2. parse manifest to get image digest per arch (sha256:XXX, sha256:YYY)
# 3. patch Dockerfile with amd64 base image (FROM ceph/ceph:v14.2@sha256:XXX)
# 4. build and push amd64 image
# 5. patch Dockerfile with arm64 base image (FROM ceph/ceph:v14.2@sha256:YYY)
# 6. build and push arm64 image
build_push_images() {
	# "docker manifest" requires experimental feature enabled
	export DOCKER_CLI_EXPERIMENTAL=enabled

	# get baseimg (ceph/ceph:tag)
	dockerfile="deploy/cephcsi/image/Dockerfile"
	baseimg=$(awk '/^FROM/ {print $NF}' "${dockerfile}")

	# get image digest per architecture
	# {
	#   "arch": "amd64",
	#   "digest": "sha256:XXX"
	# }
	# {
	#   "arch": "arm64",
	#   "digest": "sha256:YYY"
	# }
	manifests=$("${CONTAINER_CMD:-docker}" manifest inspect "${baseimg}" | jq '.manifests[] | {arch: .platform.architecture, digest: .digest}')

	# build and push per arch images
	for ARCH in amd64 arm64; do
		ifs=$IFS
		IFS=
		digest=$(awk -v ARCH=${ARCH} '{if (archfound) {print $NF; exit 0}}; {archfound=($0 ~ "arch.*"ARCH)}' <<<"${manifests}")
		IFS=$ifs
		sed -i "s|\(^FROM.*\)${baseimg}.*$|\1${baseimg}@${digest}|" "${dockerfile}"
		GOARCH=${ARCH} make push-image-cephcsi
	done
}

if [ "${TRAVIS_BRANCH}" == 'release-v2.1' ]; then
	export ENV_CSI_IMAGE_VERSION='v2.1.0'
else
	echo "!!! Branch ${TRAVIS_BRANCH} is not a deployable branch; exiting"
	exit 0 # Exiting 0 so that this isn't marked as failing
fi

if [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
	"${CONTAINER_CMD:-docker}" login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io

	set -xe

	build_push_images

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
