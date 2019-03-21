#!/bin/bash

if [ "${TRAVIS_BRANCH}" == "csi-v0.3" ] && [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
    docker login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io
    make push-image-rbdplugin push-image-cephfsplugin
fi
