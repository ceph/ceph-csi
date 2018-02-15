#!/bin/sh

VERSION="v0.1.0-2"
SANITYTGZ="csi-sanity-${VERSION}.linux.amd64.tar.gz"

if [ ! -x $GOPATH/bin/csi-sanity ] ; then
	curl -s -L \
	https://github.com/kubernetes-csi/csi-test/releases/download/${VERSION}/${SANITYTGZ} \
	-o ${SANITYTGZ} && \
	tar xzvf ${SANITYTGZ} -C /tmp && \
	rm -f ${SANITYTGZ} && \
	cp /tmp/csi-sanity/csi-sanity $GOPATH/bin/csi-sanity && \
	rm -rf /tmp/csi-sanity
fi

