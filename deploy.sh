#!/bin/bash

# shellcheck source=scripts/build_step.inc.sh
source "$(dirname "${0}")/scripts/build_step.inc.sh"
source "$(dirname "${0}")/build.env"

push_helm_charts() {
	PACKAGE=$1
	CHARTDIR=$2
	VERSION=${CSI_IMAGE_VERSION//v/} # Set version (without v prefix)

	# update information in Chart.yaml if the branch is not devel
	if [ "$TRAVIS_BRANCH" != "devel" ]; then
		# Replace appVersion: canary and version: *-canary with the actual version
		sed -i "s/\(\s.*canary\)/ $VERSION/" "charts/ceph-csi-$PACKAGE/Chart.yaml"

		if [[ "$VERSION" == *"canary"* ]]; then
			# Replace devel with the version branch
			sed -i "s/devel/$TRAVIS_BRANCH/" "charts/ceph-csi-$PACKAGE/Chart.yaml"
		else
			# This is not a canary release, replace devel with the tagged branch
			sed -i "s/devel/v$VERSION/" "charts/ceph-csi-$PACKAGE/templates/NOTES.txt"
			sed -i "s/devel/v$VERSION/" "charts/ceph-csi-$PACKAGE/Chart.yaml"

		fi
	fi

	mkdir -p "$CHARTDIR/csi-charts/docs/$PACKAGE"
	cp -R "./charts/ceph-csi-$PACKAGE" "$CHARTDIR/csi-charts/docs/$PACKAGE"
	pushd "$CHARTDIR/csi-charts/docs/$PACKAGE" >/dev/null
	helm init --client-only
	helm package "ceph-csi-$PACKAGE"
	popd >/dev/null

	pushd "$CHARTDIR/csi-charts/docs" >/dev/null
	helm repo index .
	git add --all :/ && git commit -m "Update for helm charts $PACKAGE-$VERSION"
	git push https://"$GITHUB_TOKEN"@github.com/ceph/csi-charts
	popd >/dev/null

}

# Build and push images. Steps as below:
# 1. get base image from ./build.env (BASE_IMAGE=ceph/ceph:v14.2)
# 2. parse manifest to get image digest per arch (sha256:XXX, sha256:YYY)
# 3. patch Dockerfile with amd64 base image (FROM ceph/ceph:v14.2@sha256:XXX)
# 4. build and push amd64 image
# 5. patch Dockerfile with arm64 base image (FROM ceph/ceph:v14.2@sha256:YYY)
# 6. build and push arm64 image
build_push_images() {
	# "docker manifest" requires experimental feature enabled
	export DOCKER_CLI_EXPERIMENTAL=enabled

	build_env="build.env"
	baseimg=$(awk -F = '/^BASE_IMAGE=/ {print $NF}' "${build_env}")

	# get image digest per architecture
	# {
	#   "arch": "amd64",
	#   "digest": "sha256:XXX"
	# }
	# {
	#   "arch": "arm64",
	#   "digest": "sha256:YYY"
	# }
	manifests=$(docker manifest inspect "${baseimg}" | jq '.manifests[] | {arch: .platform.architecture, digest: .digest}')
	# qemu-user-static is to enable an execution of different multi-architecture containers by QEMU
	# more info at https://github.com/multiarch/qemu-user-static
	build_step "docker run multiarch/qemu-user-static container"
	docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
	# build and push per arch images
	for ARCH in amd64 arm64; do
		ifs=$IFS
		IFS=
		digest=$(awk -v ARCH=${ARCH} '{if (archfound) {print $NF; exit 0}}; {archfound=($0 ~ "arch.*"ARCH)}' <<<"${manifests}")
		IFS=$ifs
		base_image=${baseimg}@${digest}
		build_step "make push-image-cephcsi for ${ARCH}"
		GOARCH=${ARCH} BASE_IMAGE=${base_image} make push-image-cephcsi
		build_step_log "done: make push-image-cephcsi for ${ARCH} (ret=${?})"
		GOARCH=${ARCH} make create-manifest
	done
	make push-manifest
}

if [ "${TRAVIS_BRANCH}" != 'devel' ]; then
	echo "!!! Branch ${TRAVIS_BRANCH} is not a deployable branch; exiting"
	exit 0 # Exiting 0 so that this isn't marked as failing
fi

if [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
	build_step "log in to quay.io as user ${QUAY_IO_USERNAME}"
	# This is a workaround to fix docker permission denied issue during manifest create in Travis CI.
	# `docker manifest create` fails due to permission denied on `/etc/docker/certs.d/quay.io`
	# (https://github.com/docker/for-linux/issues/396).
	sudo chmod o+x /etc/docker

	"${CONTAINER_CMD:-docker}" login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io

	set -xe

	build_push_images

	CSI_CHARTS_DIR=$(mktemp -d)

	pushd "$CSI_CHARTS_DIR" >/dev/null

	curl -L https://git.io/get_helm.sh | bash

	build_step "cloning ceph/csi-charts repository"
	git clone https://github.com/ceph/csi-charts

	mkdir -p csi-charts/docs
	popd >/dev/null

	build_step "pushing RBD helm charts"
	push_helm_charts rbd "$CSI_CHARTS_DIR"
	build_step "pushing CephFS helm charts"
	push_helm_charts cephfs "$CSI_CHARTS_DIR"
	build_step_log "finished deployment!"

	[ -n "${CSI_CHARTS_DIR}" ] && rm -rf "${CSI_CHARTS_DIR}"
fi
