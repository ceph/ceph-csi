#!/bin/bash

if [ "${TRAVIS_BRANCH}" == 'master' ]; then
	export RBD_IMAGE_VERSION='v0.3.0';
	export CEPHFS_IMAGE_VERSION='v0.3.0';
elif [ "${TRAVIS_BRANCH}" == 'csi-v1.0' ]; then
	export RBD_IMAGE_VERSION='v1.0.0';
	export CEPHFS_IMAGE_VERSION='v1.0.0';
else
	echo "!!! Branch ${TRAVIS_BRANCH} is not a deployable branch; exiting";
	exit 0; # Exiting 0 so that this isn't marked as failing
fi;

if [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
	docker login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io
	make push-image-rbdplugin push-image-cephfsplugin
fi;

