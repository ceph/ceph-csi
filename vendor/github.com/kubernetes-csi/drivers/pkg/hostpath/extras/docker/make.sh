#!/bin/sh

PROG=hostpathplugin

docker build --rm -f Dockerfile.builder -t ${PROG}:builder .
docker run --rm --privileged -v $PWD:/host ${PROG}:builder cp /${PROG} /host/${PROG}
sudo chown $USER ${PROG}
docker build --rm -t docker.io/k8scsi/${PROG} .
docker rmi ${PROG}:builder
rm -f ${PROG}
