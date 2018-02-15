#!/bin/bash

if [ "${TRAVIS_BRANCH}" == "master" ] && [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
    docker login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io 
    make push-container
fi
