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
if [ x${TRAVIS} = x"true" ] ; then
	SKIP="WithCapacity|NodeUnpublishVolume|NodePublishVolume"
fi

# Get csi-sanity
if [ ! -x $GOPATH/bin/csi-sanity ] ; then
	go get -u github.com/kubernetes-csi/csi-test
	pushd $GOPATH/src/github.com/kubernetes-csi/csi-test/cmd/csi-sanity
	make all
	make install
	popd
#./hack/get-sanity.sh
fi

# Build
make hostpath

# Cleanup
rm -f $UDS

# Start the application in the background
sudo _output/$APP --endpoint=$CSI_ENDPOINT --nodeid=1 &
pid=$!

# Need to skip Capacity testing since hostpath does not support it
sudo $GOPATH/bin/csi-sanity $@ \
    --ginkgo.skip=${SKIP} \
    --csi.mountdir=$CSI_MOUNTPOINT \
    --csi.endpoint=$CSI_ENDPOINT ; ret=$?
sudo kill -9 $pid
sudo rm -f $UDS

if [ $ret -ne 0 ] ; then
	exit $ret
fi

exit 0
