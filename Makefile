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

.PHONY: all rbdplugin

IMAGE_NAME=quay.io/cephcsi/rbdplugin
IMAGE_VERSION=v0.2.0

all: rbdplugin

test:
	go test github.com/ceph/ceph-csi/pkg/... -cover
	go vet github.com/ceph/ceph-csi/pkg/...

rbdplugin:
	if [ ! -d ./vendor ]; then dep ensure; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o  _output/rbdplugin ./rbd

container: rbdplugin 
	cp _output/rbdplugin  deploy/docker
	docker build -t $(IMAGE_NAME):$(IMAGE_VERSION) deploy/docker

clean:
	go clean -r -x
	rm -f deploy/docker/rbdplugin
	-rm -rf _output
