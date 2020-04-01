# End-to-End Testing

- [End-to-End Testing](#end-to-end-testing)
  - [Introduction](#introduction)
  - [Install Kubernetes](#install-kubernetes)
  - [Test parameters](#test-parameters)
  - [Running E2E](#running-e2e)

## Introduction

End-to-end (e2e) in cephcsi provides a mechanism to test the end-to-end
behavior of the system, These tests will interact with live instances of ceph
cluster just like how a user would.

The primary objectives of the e2e tests are to ensure a consistent and reliable
behavior of the cephcsi code base and to catch hard-to-test bugs before
users do when unit and integration tests are insufficient.

The Test framework is designed
to install Rook, run cephcsi tests, and uninstall Rook.

The e2e test are  built on top of  [Ginkgo](http://onsi.github.io/ginkgo/) and
[Gomega](http://onsi.github.io/gomega/)

## Install Kubernetes

The cephcsi also provides a script for starting Kubernetes using
[minikube](../scripts/minikube.sh) so users can quickly spin up a Kubernetes
cluster.

the following parameters are available to configure  kubernetes cluster

| flag        | description                                                   |
| ----------- | ------------------------------------------------------------- |
| up          | Starts a local kubernetes cluster and prepare a disk for rook |
| down        | Stops a running local kubernetes cluster                      |
| clean       | Deletes a local kubernetes cluster                            |
| ssh         | Log into or run a command on a minikube machine with SSH      |
| deploy-rook | Deploy rook to minikube                                       |
| clean-rook  | Deletes a rook from minikube                                  |
| cephcsi     | Copy built docker images to kubernetes cluster                |
| k8s-sidecar | Copy kubernetes sidecar docker images to kubernetes cluster   |

following environment variables can be exported to customize kubernetes deployment

| ENV                | Description                                      | Default                                                            |
| ------------------ | ------------------------------------------------ | ------------------------------------------------------------------ |
| MINIKUBE_VERSION   | minikube version to install                      | latest                                                             |
| KUBE_VERSION       | kubernetes version to install                    | v1.14.10                                                           |
| MEMORY             | Amount of RAM allocated to the minikube VM in MB | 3000                                                               |
| VM_DRIVER          | VM driver to create virtual machine              | virtualbox                                                         |
| CEPHCSI_IMAGE_REPO | Repo URL to pull cephcsi images                  | quay.io/cephcsi                                                    |
| K8S_IMAGE_REPO     | Repo URL to pull kubernetes sidecar images       | quay.io/k8scsi                                                     |
| K8S_FEATURE_GATES  | Feature gates to enable on kubernetes cluster    | BlockVolume=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true |

- creating kubernetes  cluster

```console
$./minikube.sh up
```

- Teardown kubernetes cluster

```console
$./minikube.sh clean
```

## Test parameters

In addition to standard go tests parameters, the following custom parameters
are available while running tests:

| flag              | description                                                                   |
| ----------------- | ----------------------------------------------------------------------------- |
| deploy-timeout    | Timeout to wait for created kubernetes resources (default: 10 minutes)        |
| deploy-cephfs     | Deploy cephfs csi driver as part of E2E (default: true)                       |
| deploy-rbd        | Deploy rbd csi driver as part of E2E (default: true)                          |
| cephcsi-namespace | The namespace in which cephcsi driver will be created (default: "default")    |
| rook-namespace    | The namespace in which rook operator is installed (default: "rook-ceph")      |
| kubeconfig        | Path to kubeconfig containing embedded authinfo (default: $HOME/.kube/config) |
| timeout           | Panic test binary after duration d (default 0, timeout disabled)              |
| v                 | Verbose: print additional output                                              |

## Running E2E

`
Note:- Prior to running the tests, you may need to copy the kubernetes configuration
file to `$HOME/.kube/config` which is required to communicate with kubernetes
cluster or you can pass `kubeconfig`flag while running tests.
`

Functional tests are run by the `go test` command.

 ```console
 $go test ./e2e/ -timeout=20m -v -mod=vendor
 ```

Functional  tests can be invoked by `make` command

```console
$make func-test TESTOPTIONS="--deploy-timeout=10  -timeout=30m -v"
```
