#!/bin/bash
#
# When an image was built with podman, it needs importing into minikube.
#

# fail when a command returns an error
set -e -o pipefail

# "minikube ssh" fails to read the image, so use standard ssh instead
podman image save "${1}" | \
    ssh \
        -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
        -l docker -i "$(minikube ssh-key)" \
        "$(minikube ip)" docker image load

