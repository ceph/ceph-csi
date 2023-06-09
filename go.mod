module github.com/ceph/ceph-csi

go 1.20

require (
	github.com/IBM/keyprotect-go-client v0.10.0
	github.com/aws/aws-sdk-go v1.44.285
	github.com/aws/aws-sdk-go-v2/service/sts v1.19.2
	github.com/ceph/ceph-csi/api v0.0.0-00010101000000-000000000000
	// TODO: API for managing subvolume metadata and snapshot metadata requires `ceph_ci_untested` build-tag
	github.com/ceph/go-ceph v0.21.0
	github.com/container-storage-interface/spec v1.8.0
	github.com/csi-addons/replication-lib-utils v0.2.0
	github.com/csi-addons/spec v0.2.1-0.20230606140122-d20966d2e444
	github.com/gemalto/kmip-go v0.0.9
	github.com/golang/protobuf v1.5.3
	github.com/google/fscrypt v0.3.4
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/vault/api v1.9.2
	github.com/kubernetes-csi/csi-lib-utils v0.13.0
	github.com/kubernetes-csi/external-snapshotter/client/v6 v6.2.0
	github.com/libopenstorage/secrets v0.0.0-20210908194121-a1d19aa9713a
	github.com/onsi/ginkgo/v2 v2.10.0
	github.com/onsi/gomega v1.27.8
	github.com/pkg/xattr v0.4.9
	github.com/prometheus/client_golang v1.16.0
	github.com/stretchr/testify v1.8.4
	golang.org/x/crypto v0.10.0
	golang.org/x/net v0.11.0
	golang.org/x/sys v0.9.0
	google.golang.org/grpc v1.56.0
	google.golang.org/protobuf v1.30.0
	//
	// when updating k8s.io/kubernetes, make sure to update the replace section too
	//
	k8s.io/api v0.27.2
	k8s.io/apimachinery v0.27.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/cloud-provider v0.27.2
	k8s.io/klog/v2 v2.100.1
	k8s.io/kubernetes v1.27.3
	k8s.io/mount-utils v0.27.2
	k8s.io/pod-security-admission v0.0.0
	k8s.io/utils v0.0.0-20230209194617-a36077c30491
	sigs.k8s.io/controller-runtime v0.15.1-0.20230524200249-30eae58f1b98
)

require (
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/ansel1/merry v1.6.2 // indirect
	github.com/ansel1/merry/v2 v2.0.1 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v1.4.10 // indirect
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/aws/aws-sdk-go-v2 v1.18.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.28 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.4.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/frankban/quicktest v1.13.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gemalto/flume v0.13.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // indirect
	github.com/go-jose/go-jose/v3 v3.0.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/cel-go v0.12.6 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.11.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.2.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.0 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.6 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-3 // indirect
	github.com/hashicorp/vault v1.11.11 // indirect
	github.com/hashicorp/vault/sdk v0.7.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/selinux v1.10.0 // indirect
	github.com/openshift/api v0.0.0-20230320192226-1fc631efd341 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/spf13/cobra v1.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.etcd.io/etcd/api/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/v3 v3.5.7 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.35.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.35.1 // indirect
	go.opentelemetry.io/otel v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.10.0 // indirect
	go.opentelemetry.io/otel/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.10.0 // indirect
	go.opentelemetry.io/otel/trace v1.10.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/oauth2 v0.7.0 // indirect
	golang.org/x/sync v0.2.0 // indirect
	golang.org/x/term v0.9.0 // indirect
	golang.org/x/text v0.10.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.9.3 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/api v0.110.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.27.2 // indirect
	k8s.io/apiserver v0.27.2 // indirect
	k8s.io/component-base v0.27.2 // indirect
	k8s.io/component-helpers v0.27.2 // indirect
	k8s.io/controller-manager v0.27.2 // indirect
	k8s.io/kms v0.27.2 // indirect
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f // indirect
	k8s.io/kubectl v0.0.0 // indirect
	k8s.io/kubelet v0.0.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.1.2 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	code.cloudfoundry.org/gofileutils => github.com/cloudfoundry/gofileutils v0.0.0-20170111115228-4d0c80011a0f
	github.com/ceph/ceph-csi/api => ./api

	// Required for kubernetes 1.26
	github.com/portworx/sched-ops => github.com/portworx/sched-ops v0.20.4-openstorage-rc3
	gomodules.xyz/jsonpatch/v2 => github.com/gomodules/jsonpatch/v2 v2.2.0
	//
	// k8s.io/kubernetes depends on these k8s.io packages, but unversioned
	//
	k8s.io/api => k8s.io/api v0.27.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.27.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.27.2
	k8s.io/apiserver => k8s.io/apiserver v0.27.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.27.2
	k8s.io/client-go => k8s.io/client-go v0.27.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.27.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.27.2
	k8s.io/code-generator => k8s.io/code-generator v0.27.2
	k8s.io/component-base => k8s.io/component-base v0.27.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.27.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.27.2
	k8s.io/cri-api => k8s.io/cri-api v0.27.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.27.2
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.27.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.27.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.27.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.27.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.27.2
	k8s.io/kubectl => k8s.io/kubectl v0.27.2
	k8s.io/kubelet => k8s.io/kubelet v0.27.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.27.2
	k8s.io/metrics => k8s.io/metrics v0.27.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.27.2
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.27.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.27.2
	// layeh.com seems to be misbehaving
	layeh.com/radius => github.com/layeh/radius v0.0.0-20190322222518-890bc1058917
)

exclude (
	// missing tag, referred to by github.com/hashicorp/go-kms-wrapping@v0.5.1
	github.com/hashicorp/vault/sdk v0.1.14-0.20191229212425-c478d00be0d6

	// This tag doesn't exist, but is imported by github.com/portworx/sched-ops.
	github.com/kubernetes-incubator/external-storage v0.20.4-openstorage-rc2

	// version 3.9 is really old, don't use that!
	github.com/openshift/api v3.9.0+incompatible
)
