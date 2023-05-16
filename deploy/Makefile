# Copyright 2021 The Ceph-CSI Authors.
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

.PHONY: all
all: \
	scc.yaml \
	cephfs/kubernetes/csidriver.yaml \
	cephfs/kubernetes/csi-config-map.yaml \
	nfs/kubernetes/csidriver.yaml \
	nfs/kubernetes/csi-config-map.yaml \
	rbd/kubernetes/csidriver.yaml \
	rbd/kubernetes/csi-config-map.yaml

scc.yaml: ../api/deploy/ocp/scc.yaml ../api/deploy/ocp/scc.go
	$(MAKE) -C ../tools generate-deploy

cephfs/kubernetes/csidriver.yaml: ../api/deploy/kubernetes/cephfs/csidriver.yaml ../api/deploy/kubernetes/cephfs/csidriver.go
	$(MAKE) -C ../tools generate-deploy

cephfs/kubernetes/csi-config-map.yaml: ../api/deploy/kubernetes/cephfs/csi-config-map.*
	$(MAKE) -C ../tools generate-deploy

nfs/kubernetes/csidriver.yaml: ../api/deploy/kubernetes/nfs/csidriver.yaml ../api/deploy/kubernetes/nfs/csidriver.go
	$(MAKE) -C ../tools generate-deploy

nfs/kubernetes/csi-config-map.yaml: ../api/deploy/kubernetes/nfs/csi-config-map.*
	$(MAKE) -C ../tools generate-deploy

rbd/kubernetes/csidriver.yaml: ../api/deploy/kubernetes/rbd/csidriver.yaml ../api/deploy/kubernetes/rbd/csidriver.go
	$(MAKE) -C ../tools generate-deploy

rbd/kubernetes/csi-config-map.yaml: ../api/deploy/kubernetes/rbd/csidriver.*
	$(MAKE) -C ../tools generate-deploy
