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
CSI_IMAGE_VERSION=$(if $(ENV_CSI_IMAGE_VERSION),$(ENV_CSI_IMAGE_VERSION),v2.1-canary)
CSI_IMAGE=$(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION)

$(info cephcsi image settings: $(CSI_IMAGE_NAME) version $(CSI_IMAGE_VERSION))
ifndef GIT_COMMIT
GIT_COMMIT=$(shell git rev-list -1 HEAD)
endif

GO_PROJECT=github.com/ceph/ceph-csi

# go build flags
LDFLAGS ?=
LDFLAGS += -X $(GO_PROJECT)/pkg/util.GitCommit=$(GIT_COMMIT)
# CSI_IMAGE_VERSION will be considered as the driver version
LDFLAGS += -X $(GO_PROJECT)/pkg/util.DriverVersion=$(CSI_IMAGE_VERSION)

# set GOARCH explicitly for cross building, default to native architecture
ifndef GOARCH
GOARCH := $(shell go env GOARCH)
endif

ifdef BASE_IMAGE
BASE_IMAGE_ARG = --build-arg BASE_IMAGE=$(BASE_IMAGE)
endif

SELINUX := $(shell getenforce 2>/dev/null)
ifeq ($(SELINUX),Enforcing)
	SELINUX_VOL_FLAG = :z
endif

all: cephcsi

test: go-test static-check mod-check

go-test:
	./scripts/test-go.sh

mod-check:
	@echo 'running: go mod verify'
	@go mod verify && [ "$(shell sha512sum go.mod)" = "`sha512sum go.mod`" ] || ( echo "ERROR: go.mod was modified by 'go mod verify'" && false )

static-check:
	./scripts/lint-go.sh
	./scripts/lint-text.sh --require-all
	./scripts/gosec.sh

func-test:
	go test -mod=vendor github.com/ceph/ceph-csi/e2e $(TESTOPTIONS)

.PHONY: cephcsi
cephcsi:
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
	GOOS=linux GOARCH=$(GOARCH) go build -mod vendor -a -ldflags '$(LDFLAGS)' -o _output/cephcsi ./cmd/

.PHONY: containerized-build
containerized-build: .devel-container-id
	$(CONTAINER_CMD) run --rm -v $(PWD):/go/src/github.com/ceph/ceph-csi$(SELINUX_VOL_FLAG) $(CSI_IMAGE_NAME):devel make -C /go/src/github.com/ceph/ceph-csi cephcsi

# create a (cached) container image with dependencied for building cephcsi
.devel-container-id: scripts/Dockerfile.devel
	[ ! -f .devel-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):devel
	$(CONTAINER_CMD) build -t $(CSI_IMAGE_NAME):devel -f ./scripts/Dockerfile.devel .
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):devel > .devel-container-id

image-cephcsi:
	$(CONTAINER_CMD) build -t $(CSI_IMAGE) -f deploy/cephcsi/image/Dockerfile . --build-arg GOLANG_VERSION=1.13.8 --build-arg CSI_IMAGE_NAME=$(CSI_IMAGE_NAME) --build-arg CSI_IMAGE_VERSION=$(CSI_IMAGE_VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg GO_ARCH=$(GOARCH) $(BASE_IMAGE_ARG)

push-image-cephcsi: image-cephcsi
	$(CONTAINER_CMD) tag $(CSI_IMAGE) $(CSI_IMAGE)-$(GOARCH)
	$(CONTAINER_CMD) push $(CSI_IMAGE)-$(GOARCH)
	# push amd64 image as default one
	if [ $(GOARCH) = amd64 ]; then $(CONTAINER_CMD) push $(CSI_IMAGE); fi

clean:
	go clean -r -x
	rm -f deploy/cephcsi/image/cephcsi
	rm -f _output/cephcsi
	[ ! -f .devel-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):devel
	$(RM) .devel-container-id
