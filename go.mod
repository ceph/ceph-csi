module github.com/ceph/ceph-csi

go 1.13

require (
	github.com/ceph/go-ceph v0.8.0
	github.com/container-storage-interface/spec v1.3.0
	github.com/go-logr/logr v0.2.1 // indirect
	github.com/golang/protobuf v1.4.3
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.1-0.20190118093823-f849b5445de4
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/vault/api v1.0.5-0.20200902155336-f9d5ce5a171a
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	github.com/kubernetes-csi/external-snapshotter/v2 v2.1.1
	github.com/libopenstorage/secrets v0.0.0-20201006135900-af310b01fe47
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.7.1
	github.com/stretchr/testify v1.7.0
	golang.org/x/sys v0.0.0-20201112073958-5cba982894dd
	google.golang.org/grpc v1.29.1
	k8s.io/api v0.20.0
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/cloud-provider v0.20.0
	k8s.io/klog/v2 v2.4.0
	k8s.io/kubernetes v1.20.0
	k8s.io/mount-utils v0.20.0
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/hashicorp/vault/api => github.com/hashicorp/vault/api v1.0.5-0.20200902155336-f9d5ce5a171a
	github.com/hashicorp/vault/sdk => github.com/hashicorp/vault/sdk v0.1.14-0.20201116234512-b4d4137dfe8b
	github.com/kubernetes-csi/external-snapshotter/v2 => github.com/kubernetes-csi/external-snapshotter/v2 v2.1.1-0.20200504125226-859696c419ff
	github.com/kubernetes-incubator/external-storage => github.com/kubernetes-incubator/external-storage v5.5.0+incompatible
	google.golang.org/grpc => google.golang.org/grpc v1.26.0
	k8s.io/api => k8s.io/api v0.20.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.0
	k8s.io/apiserver => k8s.io/apiserver v0.20.0
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.0
	k8s.io/client-go => k8s.io/client-go v0.20.0
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.0
	k8s.io/code-generator => k8s.io/code-generator v0.20.0
	k8s.io/component-base => k8s.io/component-base v0.20.0
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.0
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.0
	k8s.io/cri-api => k8s.io/cri-api v0.20.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.0
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.0
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.0
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.0
	k8s.io/kubectl => k8s.io/kubectl v0.20.0
	k8s.io/kubelet => k8s.io/kubelet v0.20.0
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.0
	k8s.io/metrics => k8s.io/metrics v0.20.0
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.0
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.0
	vbom.ml/util => github.com/fvbommel/util v0.0.0-20180919145318-efcd4e0f9787
)
