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
finalimg=$(awk -F = '/^FINAL_BASE_IMAGE=/ {print $NF}' "${build_env}")

# get image digest per architecture for the builder base and the final
# (runtime) base image. Both need to be pinned to the matching arch digest,
# otherwise the final stage falls back to the host arch and the resulting
# image is labelled with the wrong architecture.
# {
#   "arch": "amd64",
#   "digest": "sha256:XYZ"
# }
# {
#   "arch": "arm64",
#   "digest": "sha256:ZYX"
# }
manifests=$(docker manifest inspect "${baseimg}" | jq '.manifests[] | {arch: .platform.architecture, digest: .digest}')
final_manifests=$(docker manifest inspect "${finalimg}" | jq '.manifests[] | {arch: .platform.architecture, digest: .digest}')
# qemu-user-static is to enable an execution of different multi-architecture containers by QEMU
# more info at https://github.com/multiarch/qemu-user-static
build_step "starting multiarch/qemu-user-static container"
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
# build and push per arch images
for ARCH in amd64 arm64; do
	ifs=$IFS
	IFS=
	digest=$(awk -v ARCH=${ARCH} '{if (archfound) {print $NF; exit 0}}; {archfound=($0 ~ "arch.*"ARCH)}' <<<"${manifests}")
	final_digest=$(awk -v ARCH=${ARCH} '{if (archfound) {print $NF; exit 0}}; {archfound=($0 ~ "arch.*"ARCH)}' <<<"${final_manifests}")
	IFS=$ifs
	base_img=${baseimg}@${digest}
	final_base_img=${finalimg}@${final_digest}
	build_step "make image-cephcsi for ${ARCH}"
	GOARCH=${ARCH} BASE_IMAGE=${base_img} FINAL_BASE_IMAGE=${final_base_img} make image-cephcsi
done
