#!/bin/bash

# shellcheck source=scripts/build_step.inc.sh
source "$(dirname "${0}")/build_step.inc.sh"

set -xe
# "docker manifest" requires experimental feature enabled
export DOCKER_CLI_EXPERIMENTAL=enabled

cd "$(dirname "${0}")/.."

# ceph base image used for building multi architecture images
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
build_step "starting multiarch/qemu-user-static container"
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
# build and push per arch images
for ARCH in amd64 arm64; do
	ifs=$IFS
	IFS=
	digest=$(awk -v ARCH=${ARCH} '{if (archfound) {print $NF; exit 0}}; {archfound=($0 ~ "arch.*"ARCH)}' <<<"${manifests}")
	IFS=$ifs
	base_img=${baseimg}@${digest}
	build_step "make image-cephcsi for ${ARCH}"
	GOARCH=${ARCH} BASE_IMAGE=${base_img} make image-cephcsi
done
