module github.com/ceph/ceph-csi

go 1.16

require (
	github.com/aws/aws-sdk-go v1.41.0
	github.com/ceph/ceph-csi/api v0.0.0-00010101000000-000000000000
	github.com/ceph/go-ceph v0.11.0
	github.com/container-storage-interface/spec v1.5.0
	github.com/csi-addons/replication-lib-utils v0.2.0
	github.com/csi-addons/spec v0.1.1
	github.com/golang/protobuf v1.5.2
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/vault/api v1.1.1
	github.com/kubernetes-csi/csi-lib-utils v0.10.0
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/libopenstorage/secrets v0.0.0-20210908194121-a1d19aa9713a
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.16.0
	github.com/pborman/uuid v1.2.1
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/sys v0.0.0-20210817190340-bfb29a6856f2
	google.golang.org/grpc v1.41.0
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/cloud-provider v0.22.2
	k8s.io/klog/v2 v2.10.0
	k8s.io/kubernetes v1.22.2
	k8s.io/mount-utils v0.22.2
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/controller-runtime v0.10.2
)

replace (
	code.cloudfoundry.org/gofileutils => github.com/cloudfoundry/gofileutils v0.0.0-20170111115228-4d0c80011a0f
	github.com/ceph/ceph-csi/api => ./api
	github.com/golang/protobuf => github.com/golang/protobuf v1.4.3
	github.com/hashicorp/vault/sdk => github.com/hashicorp/vault/sdk v0.1.14-0.20201116234512-b4d4137dfe8b
	github.com/portworx/sched-ops => github.com/portworx/sched-ops v0.20.4-openstorage-rc3
	gomodules.xyz/jsonpatch/v2 => github.com/gomodules/jsonpatch/v2 v2.2.0
	//
	// k8s.io/kubernetes depends on these k8s.io packages, but unversioned
	//
	k8s.io/api => k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.2
	k8s.io/apiserver => k8s.io/apiserver v0.22.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.2
	k8s.io/client-go => k8s.io/client-go v0.22.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.2
	k8s.io/code-generator => k8s.io/code-generator v0.22.2
	k8s.io/component-base => k8s.io/component-base v0.22.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.22.2
	k8s.io/cri-api => k8s.io/cri-api v0.22.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.2
	k8s.io/kubectl => k8s.io/kubectl v0.22.2
	k8s.io/kubelet => k8s.io/kubelet v0.22.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.2
	k8s.io/metrics => k8s.io/metrics v0.22.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.22.2
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.22.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.2
)

// This tag doesn't exist, but is imported by github.com/portworx/sched-ops.
exclude github.com/kubernetes-incubator/external-storage v0.20.4-openstorage-rc2
