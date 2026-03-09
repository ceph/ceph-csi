module github.com/ceph/ceph-csi

go 1.25.0

// our own API
replace github.com/ceph/ceph-csi/api => ./api

require (
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.4.0
	github.com/IBM/keyprotect-go-client v0.15.1
	github.com/avast/retry-go/v4 v4.7.0
	github.com/aws/aws-sdk-go v1.55.8
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.8
	github.com/ceph/ceph-csi/api v0.0.0-00010101000000-000000000000
	github.com/ceph/ceph-nvmeof/lib/go/nvmeof v0.0.0-20260120065657-2425981cdad5
	github.com/ceph/go-ceph v0.38.0
	github.com/container-storage-interface/spec v1.12.0
	github.com/csi-addons/kubernetes-csi-addons v0.14.0
	github.com/csi-addons/spec v0.2.1-0.20250610152019-b5a7205f6a79
	github.com/gemalto/kmip-go v0.1.0
	github.com/golang/protobuf v1.5.4
	github.com/google/fscrypt v0.3.6
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/hashicorp/vault/api v1.22.0
	github.com/kubernetes-csi/csi-lib-utils v0.23.2
	github.com/libopenstorage/secrets v0.0.0-20231011182615-5f4b25ceede1
	github.com/pkg/xattr v0.4.12
	github.com/prometheus/client_golang v1.23.2
	github.com/stretchr/testify v1.11.1
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/crypto v0.48.0
	golang.org/x/net v0.50.0
	golang.org/x/sys v0.41.0
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
	k8s.io/api v0.35.1
	k8s.io/apimachinery v0.35.1
	k8s.io/cloud-provider v0.35.1
	k8s.io/klog/v2 v2.140.0
	k8s.io/kubernetes v1.35.2
	k8s.io/mount-utils v0.35.1
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4
)

require (
	// sigs.k8s.io/controller-runtime wants this version, it gets replaced below
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.23.3
)

replace k8s.io/client-go => k8s.io/client-go v0.35.1

exclude (
	// missing tag, referred to by github.com/hashicorp/go-kms-wrapping@v0.5.1
	github.com/hashicorp/vault/sdk v0.1.14-0.20191229212425-c478d00be0d6

	// version 3.9 is really old, don't use that!
	github.com/openshift/api v3.9.0+incompatible

	// this tag does not exist, but github.com/libopenstorage/secrets points to it
	github.com/portworx/sched-ops v1.20.4-rc1
)

require (
	cyphar.com/go-pathrs v0.2.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.20.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/ansel1/merry v1.8.1 // indirect
	github.com/ansel1/merry/v2 v2.2.2 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.19 // indirect
	github.com/aws/smithy-go v1.24.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cyphar/filepath-securejoin v0.6.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/gemalto/flume v1.0.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/hashicorp/vault/api/auth/approle v0.5.0 // indirect
	github.com/hashicorp/vault/api/auth/kubernetes v0.5.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/selinux v1.13.0 // indirect
	github.com/openshift/api v0.0.0-20240115183315-0793e918179d // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/term v0.40.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.35.1 // indirect
	k8s.io/apiserver v0.35.1 // indirect
	k8s.io/component-base v0.35.1 // indirect
	k8s.io/controller-manager v0.35.1 // indirect
	k8s.io/csi-translation-lib v0.35.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2-0.20260122202528-d9cc6641c482 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
