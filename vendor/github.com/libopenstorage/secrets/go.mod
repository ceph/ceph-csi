module github.com/libopenstorage/secrets

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v36.2.0+incompatible
	github.com/Azure/go-autorest/autorest v0.9.2
	github.com/Azure/go-autorest/autorest/adal v0.7.0
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/IBM/keyprotect-go-client v0.5.1
	github.com/aws/aws-sdk-go v1.25.41
	github.com/golang/mock v1.4.3
	github.com/hashicorp/go-hclog v0.14.1
	github.com/hashicorp/vault v1.4.2
	github.com/hashicorp/vault/api v1.0.5-0.20200902155336-f9d5ce5a171a
	github.com/pborman/uuid v1.2.0
	github.com/portworx/dcos-secrets v0.0.0-20180616013705-8e8ec3f66611
	github.com/portworx/kvdb v0.0.0-20200929023115-b312c7519467
	github.com/portworx/sched-ops v0.0.0-20200831185134-3e8010dc7056
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.4.0
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	google.golang.org/api v0.14.0
	google.golang.org/protobuf v1.25.0 // indirect
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	github.com/hashicorp/consul => github.com/hashicorp/consul v1.5.1
	github.com/kubernetes-incubator/external-storage => github.com/libopenstorage/external-storage v5.1.0-openstorage+incompatible
	github.com/kubernetes-incubator/external-storage v0.0.0-00010101000000-000000000000 => github.com/libopenstorage/external-storage v5.1.0-openstorage+incompatible
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v1.8.2-0.20190424153033-d3245f150225
	k8s.io/api => k8s.io/api v0.15.11
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.15.11
	k8s.io/apimachinery => k8s.io/apimachinery v0.15.11
	k8s.io/apiserver => k8s.io/apiserver v0.15.11
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.15.11
	k8s.io/client-go => k8s.io/client-go v0.15.11
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.15.11
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.15.11
	k8s.io/code-generator => k8s.io/code-generator v0.15.11
	k8s.io/component-base => k8s.io/component-base v0.15.11
	k8s.io/cri-api => k8s.io/cri-api v0.15.11
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.15.11
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.15.11
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.15.11
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.15.11
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.15.11
	k8s.io/kubectl => k8s.io/kubectl v0.15.11
	k8s.io/kubelet => k8s.io/kubelet v0.15.11
	k8s.io/kubernetes => k8s.io/kubernetes v1.16.0
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.15.11
	k8s.io/metrics => k8s.io/metrics v0.15.11
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.15.11
)
