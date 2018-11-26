#!/bin/sh

VERSION="v1.0.0-rc2"
SANITYTGZ="csi-sanity-${VERSION}.linux.amd64.tar.gz"

echo "Downloading csi-test from https://github.com/kubernetes-csi/csi-test/releases/download/${VERSION}/${SANITYTGZ}"
curl -s -L "https://github.com/kubernetes-csi/csi-test/releases/download/${VERSION}/${SANITYTGZ}" -o ${SANITYTGZ}
tar xzvf ${SANITYTGZ} -C /tmp && \
rm -f ${SANITYTGZ} && \
rm -f $GOPATH/bin/csi-sanity
cp /tmp/csi-sanity/csi-sanity $GOPATH/bin/csi-sanity && \
rm -rf /tmp/csi-sanity
