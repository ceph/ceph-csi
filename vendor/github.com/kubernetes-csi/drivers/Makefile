# Copyright 2017 The Kubernetes Authors.
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

REGISTRY_NAME = quay.io/k8scsi
IMAGE_VERSION = v0.2.0

.PHONY: all flexadapter nfs hostpath iscsi cinder clean

all: flexadapter nfs hostpath iscsi cinder

test:
	go test github.com/kubernetes-csi/drivers/pkg/... -cover
	go vet github.com/kubernetes-csi/drivers/pkg/...

flexadapter:
	if [ ! -d ./vendor ]; then dep ensure; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o _output/flexadapter ./app/flexadapter
nfs:
	if [ ! -d ./vendor ]; then dep ensure; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o _output/nfsplugin ./app/nfsplugin
hostpath:
	if [ ! -d ./vendor ]; then dep ensure; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o _output/hostpathplugin ./app/hostpathplugin

hostpath-container: hostpath
	cp _output/hostpathplugin pkg/hostpath/extras/docker 
	docker build -t $(REGISTRY_NAME)/hostpathplugin:$(IMAGE_VERSION) ./pkg/hostpath/extras/docker

iscsi:
	if [ ! -d ./vendor ]; then dep ensure; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o _output/iscsiplugin ./app/iscsiplugin
cinder:
	if [ ! -d ./vendor ]; then dep ensure; fi
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o _output/cinderplugin ./app/cinderplugin

clean:
	go clean -r -x
	-rm -rf _output
