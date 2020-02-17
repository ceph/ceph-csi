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
CSI_IMAGE=$(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION)

$(info cephcsi image settings: $(CSI_IMAGE_NAME) version $(CSI_IMAGE_VERSION))

GIT_COMMIT=$(shell git rev-list -1 HEAD)

GO_PROJECT=github.com/ceph/ceph-csi

# go build flags
LDFLAGS ?=
LDFLAGS += -X $(GO_PROJECT)/pkg/util.GitCommit=$(GIT_COMMIT)
# CSI_IMAGE_VERSION will be considered as the driver version
LDFLAGS += -X $(GO_PROJECT)/pkg/util.DriverVersion=$(CSI_IMAGE_VERSION)

# set GOARCH explicitly for cross building, default to native architecture
ifeq ($(origin GOARCH), undefined)
GOARCH := $(shell go env GOARCH)
endif

all: cephcsi

test: go-test static-check mod-check

go-test:
	./scripts/test-go.sh

mod-check:
	go mod verify

static-check:
	./scripts/lint-go.sh
	./scripts/lint-text.sh --require-all
	./scripts/gosec.sh

func-test:
	go test github.com/ceph/ceph-csi/e2e $(TESTOPTIONS)

.PHONY: cephcsi
cephcsi:
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
	GOOS=linux GOARCH=$(GOARCH) go build -mod vendor -a -ldflags '$(LDFLAGS)' -o _output/cephcsi ./cmd/

image-cephcsi: cephcsi
	cp _output/cephcsi deploy/cephcsi/image/cephcsi
	chmod +x deploy/cephcsi/image/cephcsi
	$(CONTAINER_CMD) build -t $(CSI_IMAGE) deploy/cephcsi/image

push-image-cephcsi: image-cephcsi
	$(CONTAINER_CMD) tag $(CSI_IMAGE) $(CSI_IMAGE)-$(GOARCH)
	$(CONTAINER_CMD) push $(CSI_IMAGE)-$(GOARCH)
	# push amd64 image as default one
	if [ $(GOARCH) = amd64 ]; then $(CONTAINER_CMD) push $(CSI_IMAGE); fi

clean:
	go clean -r -x
	rm -f deploy/cephcsi/image/cephcsi
	rm -f _output/cephcsi
