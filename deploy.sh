#!/bin/bash

if [ "${TRAVIS_BRANCH}" == "master" ] && [ "${TRAVIS_PULL_REQUEST}" == "false" ]; then
    docker login -u "${QUAY_IO_USERNAME}" -p "${QUAY_IO_PASSWORD}" quay.io 
    make push-image-rbdplugin push-image-cephfsplugin

    set -e

    mkdir -p tmp
    pushd tmp > /dev/null

    curl https://raw.githubusercontent.com/helm/helm/master/scripts/get > get_helm.sh
    chmod 700 get_helm.sh
    ./get_helm.sh

    git clone https://github.com/ceph/csi-ceph
    git clone https://github.com/ceph/ceph-csi

    mkdir -p csi-ceph/docs
    pushd ceph-csi > /dev/null
    git checkout -b csi-v1.0

    CHANGED=0
    VERSION=$(cat deploy/rbd/helm/Chart.yaml | awk '{if(/^version:/){print $2}}')

    if [ ! -f "../csi-ceph/docs/ceph-csi-rbd-$VERSION.tgz" ]; then
        CHANGED=1
        ln -s deploy/rbd/helm/ deploy/rbd/ceph-csi-rbd
	pushd ../csi-ceph/docs/ > /dev/null
        helm package ../ceph-csi/deploy/rbd/ceph-csi-rbd
        popd > /dev/null
    fi
    popd > /dev/null

    if [ $CHANGED -eq 1 ]; then
        pushd csi-ceph/docs > /dev/null
        helm repo index .
        git add --all :/ && git commit -m "Update repo"
        git push https://"$GITHUB_TOKEN"@github.com/ceph/csi-charts
        popd > /dev/null
    fi

    popd > /dev/null
fi
