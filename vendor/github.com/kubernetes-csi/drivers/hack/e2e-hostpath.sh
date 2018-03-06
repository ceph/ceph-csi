#!/bin/bash

## This file is for app/hostpathplugin
## It could be used for other apps in this repo, but
## those applications may or may not take the same
## arguments

## Must be run from the root of the repo

UDS="/tmp/e2e-csi-sanity.sock"
CSI_ENDPOINT="unix://${UDS}"
CSI_MOUNTPOINT="/mnt"
APP=hostpathplugin

SKIP="WithCapacity"
SKIP=""
if [ x${TRAVIS} = x"true" ] ; then
	SKIP="WithCapacity|NodeUnpublishVolume|NodePublishVolume"
fi

# Get csi-sanity
git clone https://github.com/kubernetes-csi/csi-test $GOPATH/src/github.com/kubernetes-csi/csi-test
pushd $GOPATH/src/github.com/kubernetes-csi/csi-test/cmd/csi-sanity
make all
make install
popd
#./hack/get-sanity.sh

# Build
cd app/hostpathplugin
  go install || exit 1
cd ../..

# Cleanup
rm -f $UDS

# Start the application in the background
sudo $GOPATH/bin/$APP --endpoint=$CSI_ENDPOINT --nodeid=1 &
pid=$!

# Need to skip Capacity testing since hostpath does not support it
sudo $GOPATH/bin/csi-sanity $@ \
    --ginkgo.skip=${SKIP} \
    --csi.mountpoint=$CSI_MOUNTPOINT \
    --csi.endpoint=$CSI_ENDPOINT ; ret=$?
sudo kill -9 $pid
sudo rm -f $UDS

if [ $ret -ne 0 ] ; then
	exit $ret
fi

exit 0
