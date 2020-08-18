# Copyright 2020 The Ceph-CSI Authors.
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

CONTAINER_CMD?=$(shell docker version >/dev/null 2>&1 && echo docker)
ifeq ($(CONTAINER_CMD),)
    CONTAINER_CMD=$(shell podman version >/dev/null 2>&1 && echo podman)
endif
CPUS?=$(shell nproc --ignore=1)
CPUSET?=--cpuset-cpus=0-${CPUS}

CSI_IMAGE_NAME=$(if $(ENV_CSI_IMAGE_NAME),$(ENV_CSI_IMAGE_NAME),quay.io/cephcsi/cephcsi)

# passing TARGET=static-check on the 'make containerized-test' commandline will
# run the selected target instead of 'make test' in the container. Obviously
# other targets can be passed as well, making it easier for developers to run
# single tests.
TARGET ?= lint-all

# Pass GIT_SINCE for the range of commits to test. Used with the commitlint
# target.
GIT_SINCE := origin/ci/centos

SELINUX := $(shell getenforce 2>/dev/null)
ifeq ($(SELINUX),Enforcing)
	SELINUX_VOL_FLAG = :z
endif

.PHONY: test
test:
	$(MAKE) containerized-test TARGET=lint-all
	$(MAKE) containerized-test TARGET=commitlint

.PHONY: lint-all lint-shell lint-markdown lint-yaml commitlint
lint-all: lint-shell lint-markdown lint-yaml

lint-shell:
	./scripts/lint-extras.sh lint-shell

lint-markdown:
	./scripts/lint-extras.sh lint-markdown

lint-yaml:
	./scripts/lint-extras.sh lint-yaml

#
# commitlint will do a rebase on top of GIT_SINCE when REBASE=1 is passed.
#
# Usage: make commitlint REBASE=1
#
commitlint: REBASE ?= 0
commitlint:
	git fetch -v $(shell cut -d/ -f1 <<< "$(GIT_SINCE)") $(shell cut -d/ -f2- <<< "$(GIT_SINCE)")
	@test $(REBASE) -eq 0 || git -c user.name=commitlint -c user.email=commitline@localhost rebase FETCH_HEAD
	commitlint --from FETCH_HEAD

.PHONY: containerized-test
containerized-test: REBASE ?= 0
containerized-test: .test-container-id
	$(CONTAINER_CMD) run --rm -v $(PWD):/go/src/github.com/ceph/ceph-csi$(SELINUX_VOL_FLAG) $(CSI_IMAGE_NAME):test make $(TARGET) GIT_SINCE=$(GIT_SINCE) REBASE=$(REBASE)

# create a (cached) container image with dependencies for testing the CI jobs
.test-container-id: scripts/Dockerfile.test
	[ ! -f .test-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):test
	$(CONTAINER_CMD) build $(CPUSET) -t $(CSI_IMAGE_NAME):test -f ./scripts/Dockerfile.test .
	$(CONTAINER_CMD) inspect -f '{{.Id}}' $(CSI_IMAGE_NAME):test > .test-container-id

clean:
	[ ! -f .test-container-id ] || $(CONTAINER_CMD) rmi $(CSI_IMAGE_NAME):test
	$(RM) .test-container-id
