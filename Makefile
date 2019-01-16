# Copyright 2018 The Kubernetes Authors.
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

.PHONY: all rbdplugin cephfsplugin

RBD_IMAGE_NAME=$(if $(ENV_RBD_IMAGE_NAME),$(ENV_RBD_IMAGE_NAME),quay.io/cephcsi/rbdplugin)
RBD_IMAGE_VERSION=$(if $(ENV_RBD_IMAGE_VERSION),$(ENV_RBD_IMAGE_VERSION),v1.0.0)

CEPHFS_IMAGE_NAME=$(if $(ENV_CEPHFS_IMAGE_NAME),$(ENV_CEPHFS_IMAGE_NAME),quay.io/cephcsi/cephfsplugin)
CEPHFS_IMAGE_VERSION=$(if $(ENV_CEPHFS_IMAGE_VERSION),$(ENV_CEPHFS_IMAGE_VERSION),v1.0.0)

$(info rbd    image settings: $(RBD_IMAGE_NAME) version $(RBD_IMAGE_VERSION))
$(info cephfs image settings: $(CEPHFS_IMAGE_NAME) version $(CEPHFS_IMAGE_VERSION))

all: rbdplugin cephfsplugin

test:
	go test github.com/ceph/ceph-csi/pkg/... -cover
	go vet github.com/ceph/ceph-csi/pkg/...

rbdplugin:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o  _output/rbdplugin ./rbd

image-rbdplugin: rbdplugin
	cp _output/rbdplugin  deploy/rbd/docker
	docker build -t $(RBD_IMAGE_NAME):$(RBD_IMAGE_VERSION) deploy/rbd/docker

cephfsplugin:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o  _output/cephfsplugin ./cephfs

image-cephfsplugin: cephfsplugin
	cp _output/cephfsplugin deploy/cephfs/docker
	docker build -t $(CEPHFS_IMAGE_NAME):$(CEPHFS_IMAGE_VERSION) deploy/cephfs/docker

push-image-rbdplugin: image-rbdplugin
	docker push $(RBD_IMAGE_NAME):$(RBD_IMAGE_VERSION)

push-image-cephfsplugin: image-cephfsplugin
	docker push $(CEPHFS_IMAGE_NAME):$(CEPHFS_IMAGE_VERSION)

clean:
	go clean -r -x
	rm -f deploy/rbd/docker/rbdplugin
	rm -f deploy/cephfs/docker/cephfsplugin
