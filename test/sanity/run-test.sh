#!/bin/bash

# Copyright 2022 The Ceph CSI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -eo pipefail

function cleanup {
  echo 'pkill -f cephcsi'
  if [ -z "$GITHUB_ACTIONS" ]
  then
    # if not running on github actions, do not use sudo
    pkill -f cephcsi
  else
    # if running on github actions, use sudo
    sudo pkill -f cephcsi
  fi
  echo 'Deleting CSI sanity test binary'
  rm -rf csi-test
}

trap cleanup EXIT

function install_csi_sanity_bin {
  echo 'Installing CSI sanity test binary...'
  mkdir -p $GOPATH/src/github.com/kubernetes-csi
  pushd $GOPATH/src/github.com/kubernetes-csi
  export GO111MODULE=off
  git clone https://github.com/kubernetes-csi/csi-test.git -b v4.3.0
  pushd csi-test/cmd/csi-sanity
  make install
  popd
  popd
}

if [[ -z "$(command -v csi-sanity)" ]]; then
	install_csi_sanity_bin
fi

readonly endpoint='unix:///csi/csi.sock'
nodeid='node.com'
rbddrivername="rook-ceph.rbd.csi.ceph.com"
cephfsdrivername="rook-ceph.cephfs.csi.ceph.com"
if [[ "$#" -gt 0 ]] && [[ -n "$1" ]]; then
  nodeid="$1"
fi

ARCH=$(uname -p)
if [[ "${ARCH}" == "x86_64" || ${ARCH} == "unknown" ]]; then
  ARCH="amd64"
fi

if [ -z "$GITHUB_ACTIONS" ]
then
  # if not running on github actions, do not use sudo
  _output/cephcsi --endpoint="$endpoint" --nodeid="$nodeid" -v=5 --type=rbd --nodeserver=true --drivername="$rbddrivername" --pidlimit=-1 --metricsport=9090 --metricspath=/metrics --enablegrpcmetrics=false
  _output/cephcsi --endpoint="$endpoint" --nodeid="$nodeid" -v=5 --type=cephfs --drivername="$cephfsdrivername" --pidlimit=-1 --metricsport=9090 --metricspath=/metrics --enablegrpcmetrics=false
else
  sudo _output/cephcsi --endpoint="$endpoint" --nodeid="$nodeid" -v=5 --type=rbd --nodeserver=true --drivername="$rbddrivername" --pidlimit=-1 --metricsport=9090 --metricspath=/metrics --enablegrpcmetrics=false
  sudo _output/cephcsi --endpoint="$endpoint" --nodeid="$nodeid" -v=5 --type=cephfs --drivername="$cephfsdrivername" --pidlimit=-1 --metricsport=9090 --metricspath=/metrics --enablegrpcmetrics=false
fi

echo 'Begin to run sanity test...'
skipTests='Controller Server|should work|should be idempotent|should remove target path'
CSI_SANITY_BIN=$GOPATH/bin/csi-sanity
if [ -z "$GITHUB_ACTIONS" ]
then
  # if not running on github actions, do not use sudo
  "$CSI_SANITY_BIN" --ginkgo.v --csi.secrets="$(pwd)/test/sanity/secrets.yaml" --csi.testvolumeparameters="$(pwd)/test/sanity/params.yaml" --csi.endpoint="$endpoint" --ginkgo.skip="$skipTests"
else
  # if running on github actions, use sudo
  sudo "$CSI_SANITY_BIN" --ginkgo.v --csi.secrets="$(pwd)/test/sanity/secrets.yaml" --csi.testvolumeparameters="$(pwd)/test/sanity/params.yaml" --csi.endpoint="$endpoint" --ginkgo.skip="$skipTests"
fi
