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

CONTAINERIZED?=no
CONTAINER_CMD?=$(shell podman version >/dev/null 2>&1 && echo podman)
ifeq ($(CONTAINER_CMD),)
    CONTAINER_CMD=$(shell docker version >/dev/null 2>&1 && echo docker)
endif

# Recent versions of Podman do not allow non-root to use --cpuset options.
# Set HAVE_CPUSET to 1 when cpuset support is available.
ifeq ($(UID),0)
    HAVE_CPUSET ?= $(shell grep -c -w cpuset /sys/fs/cgroup/cgroup.controllers 2>/dev/null)
else
    HAVE_CPUSET ?= $(shell grep -c -w cpuset /sys/fs/cgroup/user.slice/user-$(UID).slice/cgroup.controllers 2>/dev/null)
endif
ifeq ($(HAVE_CPUSET),1)
    CPUS ?= $(shell nproc --ignore=1)
    CPUSET ?= --cpuset-cpus=0-${CPUS}
endif

CSI_IMAGE_NAME=$(if $(ENV_CSI_IMAGE_NAME),$(ENV_CSI_IMAGE_NAME),quay.io/cephcsi/cephcsi)
CSI_IMAGE_VERSION=$(shell . $(CURDIR)/build.env ; echo $${CSI_IMAGE_VERSION})
CSI_IMAGE=$(CSI_IMAGE_NAME):$(CSI_IMAGE_VERSION)

# Pass USE_PULLED_IMAGE=yes to skip building a new :test or :devel image.
USE_PULLED_IMAGE?=no

$(info cephcsi image settings: $(CSI_IMAGE_NAME) version $(CSI_IMAGE_VERSION))
ifndef GIT_COMMIT
GIT_COMMIT=$(shell git rev-list -1 HEAD)
endif

GO_PROJECT=github.com/ceph/ceph-csi

CEPH_VERSION ?= $(shell . $(CURDIR)/build.env ; echo $${CEPH_VERSION})
# TODO: ceph_preview tag may be removed with go-ceph 0.17.0
# TODO: ceph_ci_untested is added for subvolume metadata (go-ceph#691) and snapshot metadata management (go-ceph#698)
GO_TAGS_LIST ?= $(CEPH_VERSION) ceph_preview ceph_ci_untested ceph_pre_quincy

# go build flags
LDFLAGS ?=
LDFLAGS += -X $(GO_PROJECT)/internal/util.GitCommit=$(GIT_COMMIT)
# CSI_IMAGE_VERSION will be considered as the driver version
LDFLAGS += -X $(GO_PROJECT)/internal/util.DriverVersion=$(CSI_IMAGE_VERSION)
GO_TAGS ?= -tags=$(shell echo $(GO_TAGS_LIST) | tr ' ' ',')

BASE_IMAGE ?= $(shell . $(CURDIR)/build.env ; echo $${BASE_IMAGE})

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
GIT_SINCE := origin/devel

SELINUX := $(shell getenforce 2>/dev/null)
ifeq ($(SELINUX),Enforcing)
	SELINUX_VOL_FLAG = :z
endif

all: cephcsi

.PHONY: go-test static-check mod-check go-lint lint-extras commitlint codespell
ifeq ($(CONTAINERIZED),no)
# include mod-check in non-containerized runs
test: go-test static-check mod-check
else
# exclude mod-check for containerized runs (CI runs it separately)
test: go-test static-check
endif
static-check: check-env codespell go-lint lint-extras

go-test: TEST_COVERAGE ?= $(shell . $(CURDIR)/build.env ; echo $${TEST_COVERAGE})
go-test: GO_COVER_DIR ?= $(shell . $(CURDIR)/build.env ; echo $${GO_COVER_DIR})
go-test: check-env
	TEST_COVERAGE="$(TEST_COVERAGE)" GO_COVER_DIR="$(GO_COVER_DIR)" GO_TAGS="$(GO_TAGS)" ./scripts/test-go.sh

go-test-api: check-env
	@pushd api && ../scripts/test-go.sh && popd

mod-check: check-env
	@echo 'running: go mod verify'
	@go mod verify && [ "$(shell sha512sum go.mod)" = "`sha512sum go.mod`" ] || ( echo "ERROR: go.mod was modified by 'go mod verify'" && false )
	@echo 'running: go list -mod=readonly -m all'
	@go list -mod=readonly -m all 1> /dev/null

scripts/golangci.yml: scripts/golangci.yml.in
	rm -f scripts/golangci.yml.buildtags.in
	for tag in $(GO_TAGS_LIST); do \
		echo "    - $$tag" >> scripts/golangci.yml.buildtags.in ; \
	done
	sed "/@@BUILD_TAGS@@/r scripts/golangci.yml.buildtags.in" scripts/golangci.yml.in | sed '/@@BUILD_TAGS@@/d' > scripts/golangci.yml

go-lint: scripts/golangci.yml
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

lint-py:
	./scripts/lint-extras.sh lint-py

func-test:
	go test $(GO_TAGS) -mod=vendor github.com/ceph/ceph-csi/e2e $(TESTOPTIONS)

check-env:
	@./scripts/check-env.sh

codespell:
	codespell --config scripts/codespell.conf
#
# commitlint will do a rebase on top of GIT_SINCE when REBASE=1 is passed.
#
# Usage: make commitlint REBASE=1
#
commitlint: REBASE ?= 0
commitlint:
	git fetch -v $(shell cut -d/ -f1 <<< "$(GIT_SINCE)") $(shell cut -d/ -f2- <<< "$(GIT_SINCE)")
	@test $(REBASE) -eq 0 || git -c user.name=commitlint -c user.email=commitline@localhost rebase FETCH_HEAD
	commitlint --verbose --from $(GIT_SINCE)

.PHONY: cephcsi
cephcsi: check-env
	if [ ! -d ./vendor ]; then (go mod tidy && go mod vendor); fi
	GOOS=linux go build $(GO_TAGS) -mod vendor -a -ldflags '$(LDFLAGS)' -o _output/cephcsi ./cmd/

e2e.test: check-env
	go test $(GO_TAGS) -mod=vendor -c ./e2e

#
# Update the generated deploy/ files when the template changed. This requires
# running 'go mod vendor' so update the API files under the vendor/ directory.
.PHONY: generate-deploy
generate-deploy:
	go mod vendor
	$(MAKE) -C deploy

#
# e2e testing by compiling e2e.test in case it does not exist and running the
# executable. The e2e.test executable is not checked as a dependency in the
# make rule, as the phony check-env causes rebuilds for each run.
#
# Usage: make run-e2e E2E_ARGS="--test-cephfs=false --test-rbd=true"
#
.PHONY: run-e2e
run-e2e: E2E_TIMEOUT ?= $(shell . $(CURDIR)/build.env ; echo $${E2E_TIMEOUT})
run-e2e: DEPLOY_TIMEOUT ?= $(shell . $(CURDIR)/build.env ; echo $${DEPLOY_TIMEOUT})
run-e2e: NAMESPACE ?= cephcsi-e2e-$(shell uuidgen | cut -d- -f1)
run-e2e:
	@test -e e2e.test || $(MAKE) e2e.test
	cd e2e && \
	../e2e.test -test.v -ginkgo.timeout="${E2E_TIMEOUT}" --deploy-timeout="${DEPLOY_TIMEOUT}" --cephcsi-namespace=$(NAMESPACE) $(E2E_ARGS)

.container-cmd:
	@test -n "$(shell which $(CONTAINER_CMD) 2>/dev/null)" || { echo "Missing container support, install Podman or Docker"; exit 1; }
	@echo "$(CONTAINER_CMD)" > .container-cmd

.PHONY: containerized-build containerized-test
containerized-build: TARGET = cephcsi
containerized-build: .container-cmd .devel-container-id
	$(CONTAINER_CMD) run --rm -v $(CURDIR):/go/src/github.com/ceph/ceph-csi$(SELINUX_VOL_FLAG) $(CSI_IMAGE_NAME):devel make $(TARGET) CONTAINERIZED=yes

containerized-test: TARGET = test
containerized-test: REBASE ?= 0
containerized-test: .container-cmd .test-container-id
	$(CONTAINER_CMD) run --rm -v $(CURDIR):/go/src/github.com/ceph/ceph-csi$(SELINUX_VOL_FLAG) $(CSI_IMAGE_NAME):test make $(TARGET) GIT_SINCE=$(GIT_SINCE) REBASE=$(REBASE) CONTAINERIZED=yes

ifeq ($(USE_PULLED_IMAGE),no)
# create a (cached) container image with dependencies for building cephcsi
.devel-container-id: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
.devel-container-id: .container-cmd scripts/Dockerfile.devel
	[ ! -f .devel-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):devel
	$(RM) .devel-container-id
	$(CONTAINER_CMD) build $(CPUSET) --build-arg BASE_IMAGE=$(BASE_IMAGE) --build-arg GOARCH=$(GOARCH) -t $(CSI_IMAGE_NAME):devel -f ./scripts/Dockerfile.devel .
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):devel > .devel-container-id
else
# create the .devel-container-id file based on pulled image
.devel-container-id: .container-cmd
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):devel > .devel-container-id
endif

ifeq ($(USE_PULLED_IMAGE),no)
.test-container-id: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
# create a (cached) container image with dependencies for testing cephcsi
.test-container-id: .container-cmd build.env scripts/Dockerfile.test
	[ ! -f .test-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):test
	$(RM) .test-container-id
	$(CONTAINER_CMD) build $(CPUSET) --build-arg GOARCH=$(GOARCH) -t $(CSI_IMAGE_NAME):test -f ./scripts/Dockerfile.test .
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):test > .test-container-id
else
# create the .test-container-id file based on the pulled image
.test-container-id: .container-cmd
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):test > .test-container-id
endif

image-cephcsi: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
image-cephcsi: .container-cmd
	$(CONTAINER_CMD) build $(CPUSET) -t $(CSI_IMAGE) -f deploy/cephcsi/image/Dockerfile . --build-arg CSI_IMAGE_NAME=$(CSI_IMAGE_NAME) --build-arg CSI_IMAGE_VERSION=$(CSI_IMAGE_VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg GO_ARCH=$(GOARCH) --build-arg BASE_IMAGE=$(BASE_IMAGE)

push-image-cephcsi: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
push-image-cephcsi: .container-cmd image-cephcsi
	$(CONTAINER_CMD) tag $(CSI_IMAGE) $(CSI_IMAGE)-$(GOARCH)
	$(CONTAINER_CMD) push $(CSI_IMAGE)-$(GOARCH)

create-manifest: GOARCH ?= $(shell go env GOARCH 2>/dev/null)
create-manifest: .container-cmd
	$(CONTAINER_CMD) manifest create $(CSI_IMAGE) --amend $(CSI_IMAGE)-$(GOARCH)

push-manifest: .container-cmd
	$(CONTAINER_CMD) manifest push  $(CSI_IMAGE)

clean:
	go clean -mod=vendor -r -x
	rm -f deploy/cephcsi/image/cephcsi
	rm -f _output/cephcsi
	$(RM) scripts/golangci.yml
	$(RM) e2e.test
	[ ! -f .devel-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):devel
	$(RM) .devel-container-id
	[ ! -f .test-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):test
	$(RM) .test-container-id
