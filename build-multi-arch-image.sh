#!/bin/bash
set -xe
# "docker manifest" requires experimental feature enabled
export DOCKER_CLI_EXPERIMENTAL=enabled

# ceph base image used for building multi architecture images
dockerfile="deploy/cephcsi/image/Dockerfile"
baseimg=$(awk -F = '/^ARG BASE_IMAGE=/ {print $NF}' "${dockerfile}")

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
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
# build and push per arch images
for ARCH in amd64 arm64; do
	ifs=$IFS
	IFS=
	digest=$(awk -v ARCH=${ARCH} '{if (archfound) {print $NF; exit 0}}; {archfound=($0 ~ "arch.*"ARCH)}' <<<"${manifests}")
	IFS=$ifs
	base_img=${baseimg}@${digest}
	GOARCH=${ARCH} BASE_IMAGE=${base_img} make image-cephcsi
done
