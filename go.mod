module github.com/ceph/ceph-csi

go 1.13

require (
	github.com/ceph/go-ceph v0.2.0
	github.com/container-storage-interface/spec v1.2.0
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20191227052852-215e87163ea7 // indirect
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.6.1
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.1.0 // indirect
	github.com/prometheus/common v0.8.0 // indirect
	github.com/prometheus/procfs v0.0.8 // indirect
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	go.uber.org/zap v1.13.0 // indirect
	golang.org/x/crypto v0.0.0-20200115085410-6d4e4cb37c7d // indirect
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f // indirect
	golang.org/x/net v0.0.0-20200114155413-6afb5195e5aa // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.0.0-20200116001909-b77594299b42
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	golang.org/x/tools v0.0.0-20200116062425-473961ec044c
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20200115191322-ca5a22157cba // indirect
	google.golang.org/grpc v1.26.0
	gopkg.in/square/go-jose.v2 v2.4.1 // indirect
	gopkg.in/yaml.v2 v2.2.7 // indirect
	honnef.co/go/tools v0.0.1-2019.2.3
	k8s.io/api v0.17.1
	k8s.io/apiextensions-apiserver v0.17.1 // indirect
	k8s.io/apimachinery v0.17.3-beta.0
	k8s.io/client-go v0.17.1
	k8s.io/cloud-provider v0.17.1
	k8s.io/cri-api v0.17.1 // indirect
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.17.1 // indirect
	k8s.io/kubernetes v1.17.1
	k8s.io/utils v0.0.0-20200109141947-94aeca20bf09
)

replace (
	k8s.io/api => k8s.io/api v0.17.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.1
	k8s.io/apiserver => k8s.io/apiserver v0.17.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.17.1
	k8s.io/client-go => k8s.io/client-go v0.17.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.17.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.17.1
	k8s.io/code-generator => k8s.io/code-generator v0.17.1
	k8s.io/component-base => k8s.io/component-base v0.17.1
	k8s.io/cri-api => k8s.io/cri-api v0.17.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.17.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.17.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.17.1
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.17.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.17.1
	k8s.io/kubectl => k8s.io/kubectl v0.17.1
	k8s.io/kubelet => k8s.io/kubelet v0.17.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.17.1
	k8s.io/metrics => k8s.io/metrics v0.17.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.17.1
)
