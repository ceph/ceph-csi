#!/bin/bash

if [ "${TRAVIS_BRANCH}" == "csi-v0.3" ] && [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
    export ENV_RBD_IMAGE_VERSION='v0.3-canary'
    export ENV_CEPHFS_IMAGE_VERSION='v0.3-canary'

    docker login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io
    make push-image-rbdplugin push-image-cephfsplugin
fi
