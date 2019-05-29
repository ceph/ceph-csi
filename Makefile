# Copyright 2018 The Ceph-CSI Authors.
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

.PHONY: all cephcsi

CONTAINER_CMD?=docker

CSI_IMAGE_NAME=$(if $(ENV_CSI_IMAGE_NAME),$(ENV_CSI_IMAGE_NAME),quay.io/cephcsi/cephcsi)
CSI_IMAGE_VERSION=$(if $(ENV_CSI_IMAGE_VERSION),$(ENV_CSI_IMAGE_VERSION),canary)

$(info cephcsi image settings: $(CSI_IMAGE_NAME) version $(CSI_IMAGE_VERSION))

all: cephcsi

test: go-test static-check

go-test:
	./scripts/test-go.sh

static-check:
	./scripts/lint-go.sh
	./scripts/lint-text.sh

func-test:
	go test github.com/ceph/ceph-csi/e2e $(TESTOPTIONS)

.PHONY: cephcsi
cephcsi:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o  _output/cephcsi ./cmd/

image-cephcsi: cephcsi
	cp _output/cephcsi deploy/cephcsi/image/cephcsi
	$(CONTAINER_CMD) build -t $(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION) deploy/cephcsi/image

push-image-cephcsi: image-cephcsi
	$(CONTAINER_CMD) push $(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION)


clean:
	go clean -r -x
	rm -f deploy/cephcsi/image/cephcsi
	rm -f _output/cephcsi
