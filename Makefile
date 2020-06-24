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

.PHONY: all cephcsi check-env

CONTAINER_CMD?=$(shell docker version >/dev/null 2>&1 && echo docker)
ifeq ($(CONTAINER_CMD),)
    CONTAINER_CMD=$(shell podman version >/dev/null 2>&1 && echo podman)
endif
CPUS?=$(shell nproc --ignore=1)
CPUSET?=--cpuset-cpus=0-${CPUS}

CSI_IMAGE_NAME=$(if $(ENV_CSI_IMAGE_NAME),$(ENV_CSI_IMAGE_NAME),quay.io/cephcsi/cephcsi)
CSI_IMAGE_VERSION=$(if $(ENV_CSI_IMAGE_VERSION),$(ENV_CSI_IMAGE_VERSION),canary)
CSI_IMAGE=$(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION)

$(info cephcsi image settings: $(CSI_IMAGE_NAME) version $(CSI_IMAGE_VERSION))
ifndef GIT_COMMIT
GIT_COMMIT=$(shell git rev-list -1 HEAD)
endif

GO_PROJECT=github.com/ceph/ceph-csi

# go build flags
LDFLAGS ?=
LDFLAGS += -X $(GO_PROJECT)/internal/util.GitCommit=$(GIT_COMMIT)
# CSI_IMAGE_VERSION will be considered as the driver version
LDFLAGS += -X $(GO_PROJECT)/internal/util.DriverVersion=$(CSI_IMAGE_VERSION)

BASE_IMAGE ?= $(shell . $(CURDIR)/build.env ; echo $${BASE_IMAGE})

ifndef CEPH_VERSION
	CEPH_VERSION = $(shell . $(CURDIR)/build.env ; echo $${CEPH_VERSION})
endif
ifdef CEPH_VERSION
	# pass -tags to go commands (for go-ceph build constraints)
	GO_TAGS = -tags=$(CEPH_VERSION)
endif

# passing TARGET=static-check on the 'make containerized-test' or 'make
# containerized-build' commandline will run the selected target instead of
# 'make test' in the container. Obviously other targets can be passed as well,
# making it easier for developers to run single tests or build different
# executables.
#
# Defaults:
#   make containerized-build TARGET=cephcsi -> runs 'make cephcsi'
#   make containerized-test TARGET=test -> runs 'make test'
#
# Other options:
#   make containerized-build TARGET=e2e.test -> runs 'make e2e.test'
#   make containerized-test TARGET=static-check -> runs 'make static-check'

# Pass GIT_SINCE for the range of commits to test. Used with the commitlint
# target.
GIT_SINCE := origin/master

SELINUX := $(shell getenforce 2>/dev/null)
ifeq ($(SELINUX),Enforcing)
	SELINUX_VOL_FLAG = :z
endif

all: cephcsi

.PHONY: go-test static-check mod-check go-lint lint-extras gosec commitlint
test: go-test static-check mod-check
static-check: check-env go-lint lint-extras gosec

go-test: TEST_COVERAGE ?= $(shell . $(CURDIR)/build.env ; echo $${TEST_COVERAGE})
go-test: GO_COVER_DIR ?= $(shell . $(CURDIR)/build.env ; echo $${GO_COVER_DIR})
go-test: check-env
	TEST_COVERAGE="$(TEST_COVERAGE)" GO_COVER_DIR="$(GO_COVER_DIR)" GO_TAGS="$(GO_TAGS)" ./scripts/test-go.sh

mod-check: check-env
	@echo 'running: go mod verify'
	@go mod verify && [ "$(shell sha512sum go.mod)" = "`sha512sum go.mod`" ] || ( echo "ERROR: go.mod was modified by 'go mod verify'" && false )

go-lint:
	./scripts/lint-go.sh

lint-extras:
	./scripts/lint-extras.sh lint-all

lint-shell:
	./scripts/lint-extras.sh lint-shell

lint-markdown:
	./scripts/lint-extras.sh lint-markdown

lint-yaml:
	./scripts/lint-extras.sh lint-yaml

lint-helm:
	./scripts/lint-extras.sh lint-helm

gosec:
	GO_TAGS="$(GO_TAGS)" ./scripts/gosec.sh

func-test:
	go test $(GO_TAGS) -mod=vendor github.com/ceph/ceph-csi/e2e $(TESTOPTIONS)

check-env:
	@./scripts/check-env.sh

commitlint:
	commitlint --from $(GIT_SINCE)

.PHONY: cephcsi
cephcsi: check-env
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
	GOOS=linux go build $(GO_TAGS) -mod vendor -a -ldflags '$(LDFLAGS)' -o _output/cephcsi ./cmd/

e2e.test: check-env
	go test $(GO_TAGS) -mod=vendor -c ./e2e

.PHONY: containerized-build containerized-test
containerized-build: TARGET = cephcsi
containerized-build: .devel-container-id
	$(CONTAINER_CMD) run --rm -v $(CURDIR):/go/src/github.com/ceph/ceph-csi$(SELINUX_VOL_FLAG) $(CSI_IMAGE_NAME):devel make $(TARGET)

containerized-test: TARGET = test
containerized-test: .test-container-id
	$(CONTAINER_CMD) run --rm -v $(CURDIR):/go/src/github.com/ceph/ceph-csi$(SELINUX_VOL_FLAG) $(CSI_IMAGE_NAME):test make $(TARGET) GIT_SINCE=$(GIT_SINCE)

# create a (cached) container image with dependencied for building cephcsi
.devel-container-id: scripts/Dockerfile.devel
	[ ! -f .devel-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):devel
	$(CONTAINER_CMD) build $(CPUSET) --build-arg BASE_IMAGE=$(BASE_IMAGE) -t $(CSI_IMAGE_NAME):devel -f ./scripts/Dockerfile.devel .
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):devel > .devel-container-id

# create a (cached) container image with dependencied for testing cephcsi
.test-container-id: build.env scripts/Dockerfile.test
	[ ! -f .test-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):test
	$(CONTAINER_CMD) build $(CPUSET) -t $(CSI_IMAGE_NAME):test -f ./scripts/Dockerfile.test .
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):test > .test-container-id

image-cephcsi: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
image-cephcsi:
	$(CONTAINER_CMD) build $(CPUSET) -t $(CSI_IMAGE) -f deploy/cephcsi/image/Dockerfile . --build-arg CSI_IMAGE_NAME=$(CSI_IMAGE_NAME) --build-arg CSI_IMAGE_VERSION=$(CSI_IMAGE_VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg GO_ARCH=$(GOARCH) --build-arg BASE_IMAGE=$(BASE_IMAGE)

push-image-cephcsi: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
push-image-cephcsi: image-cephcsi
	$(CONTAINER_CMD) tag $(CSI_IMAGE) $(CSI_IMAGE)-$(GOARCH)
	$(CONTAINER_CMD) push $(CSI_IMAGE)-$(GOARCH)
	# push amd64 image as default one
	if [ $(GOARCH) = amd64 ]; then $(CONTAINER_CMD) push $(CSI_IMAGE); fi

clean:
	go clean -mod=vendor -r -x
	rm -f deploy/cephcsi/image/cephcsi
	rm -f _output/cephcsi
	$(RM) e2e.test
	[ ! -f .devel-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):devel
	$(RM) .devel-container-id
	[ ! -f .test-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):test
	$(RM) .test-container-id
