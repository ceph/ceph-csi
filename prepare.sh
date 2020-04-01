#!/bin/sh

set -x

yum -y install git podman

git clone --single-branch --branch=master https://github.com/ceph/ceph-csi /opt/build
