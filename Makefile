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

CONTAINER_CMD?=docker

RBD_IMAGE_NAME=$(if $(ENV_RBD_IMAGE_NAME),$(ENV_RBD_IMAGE_NAME),quay.io/cephcsi/rbdplugin)
RBD_IMAGE_VERSION=$(if $(ENV_RBD_IMAGE_VERSION),$(ENV_RBD_IMAGE_VERSION),v1.0.0)

CEPHFS_IMAGE_NAME=$(if $(ENV_CEPHFS_IMAGE_NAME),$(ENV_CEPHFS_IMAGE_NAME),quay.io/cephcsi/cephfsplugin)
CEPHFS_IMAGE_VERSION=$(if $(ENV_CEPHFS_IMAGE_VERSION),$(ENV_CEPHFS_IMAGE_VERSION),v1.0.0)

$(info rbd    image settings: $(RBD_IMAGE_NAME) version $(RBD_IMAGE_VERSION))
$(info cephfs image settings: $(CEPHFS_IMAGE_NAME) version $(CEPHFS_IMAGE_VERSION))

all: rbdplugin cephfsplugin

test: go-test static-check

go-test:
	./scripts/test-go.sh

static-check:
	./scripts/lint-go.sh
	./scripts/lint-text.sh

rbdplugin:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o  _output/rbdplugin ./cmd/rbd

image-rbdplugin: rbdplugin
	cp _output/rbdplugin  deploy/rbd/docker
	$(CONTAINER_CMD) build -t $(RBD_IMAGE_NAME):$(RBD_IMAGE_VERSION) deploy/rbd/docker

cephfsplugin:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o  _output/cephfsplugin ./cmd/cephfs

image-cephfsplugin: cephfsplugin
	cp _output/cephfsplugin deploy/cephfs/docker
	$(CONTAINER_CMD) build -t $(CEPHFS_IMAGE_NAME):$(CEPHFS_IMAGE_VERSION) deploy/cephfs/docker

push-image-rbdplugin: image-rbdplugin
	$(CONTAINER_CMD) push $(RBD_IMAGE_NAME):$(RBD_IMAGE_VERSION)

push-image-cephfsplugin: image-cephfsplugin
	$(CONTAINER_CMD) push $(CEPHFS_IMAGE_NAME):$(CEPHFS_IMAGE_VERSION)

clean:
	go clean -r -x
	rm -f deploy/rbd/docker/rbdplugin
	rm -f deploy/cephfs/docker/cephfsplugin
	rm -f _output/rbdplugin
	rm -f _output/cephfsplugin
