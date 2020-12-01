#!/bin/bash
#
# When an image was built with podman, it needs importing into minikube.
#
# Some versions of minikube/docker add a "localhost/" prefix to imported
# images. In that case, the image needs to get tagged without the prefix as
# well.
#

# fail when a command returns an error
set -e -o pipefail

# "minikube ssh" fails to read the image, so use standard ssh instead
function minikube_ssh() {
    ssh \
        -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
        -l docker -i "$(minikube ssh-key)" \
        "$(minikube ip)" "${*}"
}

IMAGE="${1}"
# if IMAGE is empty, fail the script
[ -n "${IMAGE}" ]

# import the image, save response in STDOUT
STDOUT=$(podman image save "${IMAGE}" | minikube_ssh docker image load)
echo "${STDOUT}"

# check the name of the image that was imported in docker
DOCKER_IMAGE=$(awk '/Loaded image/ {print $NF}' <<< "${STDOUT}")

# strip "localhost/" from the image name
if [[ "${DOCKER_IMAGE}" =~ ^localhost/* ]]
then
    minikube_ssh docker tag "${DOCKER_IMAGE}" "${IMAGE}"
fi
