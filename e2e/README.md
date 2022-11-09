# End-to-End Testing

- [End-to-End Testing](#end-to-end-testing)
   - [Introduction](#introduction)
   - [Install Kubernetes](#install-kubernetes)
   - [Deploy Rook](#deploy-rook)
   - [Test parameters](#test-parameters)
   - [E2E for snapshot](#e2e-for-snapshot)
   - [Running E2E](#running-e2e)

## Introduction

End-to-end (e2e) in cephcsi provides a mechanism to test the end-to-end behavior
of the system, These tests will interact with live instances of ceph cluster
just like how a user would.

The primary objectives of the e2e tests are to ensure a consistent and reliable
behavior of the cephcsi code base and to catch hard-to-test bugs before users do
when unit and integration tests are insufficient.

The Test framework is designed to install Rook, run cephcsi tests, and uninstall
Rook.

The e2e test are built on top of  [Ginkgo](http://onsi.github.io/ginkgo/) and
[Gomega](http://onsi.github.io/gomega/)

## Install Kubernetes

The cephcsi also provides a script for starting Kubernetes using
[minikube](../scripts/minikube.sh) so users can quickly spin up a Kubernetes
cluster.

the following parameters are available to configure kubernetes cluster

| flag              | description                                                   |
| ----------------- | ------------------------------------------------------------- |
| up                | Starts a local kubernetes cluster and prepare a disk for rook |
| down              | Stops a running local kubernetes cluster                      |
| clean             | Deletes a local kubernetes cluster                            |
| ssh               | Log into or run a command on a minikube machine with SSH      |
| deploy-rook       | Deploy rook to minikube                                       |
| create-block-pool | Creates a rook block pool (named $ROOK_BLOCK_POOL_NAME)       |
| delete-block-pool | Deletes a rook block pool (named $ROOK_BLOCK_POOL_NAME)       |
| clean-rook        | Deletes a rook from minikube                                  |
| cephcsi           | Copy built docker images to kubernetes cluster                |
| k8s-sidecar       | Copy kubernetes sidecar docker images to kubernetes cluster   |

following environment variables can be exported to customize kubernetes
deployment

| ENV                  | Description                                      | Default                                                            |
|----------------------|--------------------------------------------------|--------------------------------------------------------------------|
| MINIKUBE_VERSION     | minikube version to install                      | latest                                                             |
| KUBE_VERSION         | kubernetes version to install                    | latest                                                             |
| MEMORY               | Amount of RAM allocated to the minikube VM in MB | 4096                                                               |
| VM_DRIVER            | VM driver to create virtual machine              | virtualbox                                                         |
| CEPHCSI_IMAGE_REPO   | Repo URL to pull cephcsi images                  | quay.io/cephcsi                                                    |
| K8S_IMAGE_REPO       | Repo URL to pull kubernetes sidecar images       | registry.k8s.io/sig-storage                                             |
| K8S_FEATURE_GATES    | Feature gates to enable on kubernetes cluster    | BlockVolume=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true |
| ROOK_BLOCK_POOL_NAME | Block pool name to create in the rook instance   | newrbdpool                                                         |

- creating kubernetes cluster

  From the ceph-csi root directory, run:

    ```console
    ./scripts/minikube.sh up
    ```

- Teardown kubernetes cluster

    ```console
    ./scripts/minikube.sh clean
    ```

## Deploy Rook

The cephcsi E2E tests expects that you already have rook running in your
cluster.

Thanks to [minikube](../scripts/minikube.sh) script for the handy `deploy-rook`
option.

```console
./scripts/minikube.sh deploy-rook
```

## Test parameters

In addition to standard go tests parameters, the following custom parameters are
available while running tests:

| flag              | description                                                                                       |
| ----------------- | ------------------------------------------------------------------------------------------------- |
| deploy-timeout    | Timeout to wait for created kubernetes resources (default: 10 minutes)                            |
| deploy-cephfs     | Deploy cephFS CSI driver as part of E2E (default: true)                                           |
| deploy-rbd        | Deploy rbd CSI driver as part of E2E (default: true)                                              |
| test-cephfs       | Test cephFS CSI driver as part of E2E (default: true)                                             |
| upgrade-testing   | Perform upgrade testing (default: false)                                                          |
| upgrade-version   | Target version for upgrade testing (default: "v3.5.1")                                            |
| test-rbd          | Test rbd CSI driver as part of E2E (default: true)                                                |
| cephcsi-namespace | The namespace in which cephcsi driver will be created (default: "default")                        |
| rook-namespace    | The namespace in which rook operator is installed (default: "rook-ceph")                          |
| kubeconfig        | Path to kubeconfig containing embedded authinfo (default: $HOME/.kube/config)                     |
| timeout           | Panic test binary after duration d (default 0, timeout disabled)                                  |
| v                 | Verbose: print additional output                                                                  |
| is-openshift      | Run in OpenShift compatibility mode, skips certain new feature tests                              |
| filesystem        | Name of the CephFS filesystem (default: "myfs")                                                   |
| clusterid         | Use the Ceph cluster id in the StorageClasses and SnapshotClasses (default: `ceph fsid` detected) |
| nfs-driver        | Name of the driver to use for provisioning NFS-volumes (default: "nfs.csi.ceph.com")              |

## E2E for snapshot

After the support for snapshot/clone has been added to ceph-csi, you need to
follow these steps before running e2e.

- Install snapshot controller and snapshot CRD

    ```console
    ./scripts/install-snapshot.sh install
    ```

  Once you are done running e2e please perform the cleanup by running following:

    ```console
    ./scripts/install-snapshot.sh cleanup
    ```

## Running E2E

`
Note:- Prior to running the tests, you may need to copy the kubernetes
configuration file to `$HOME/.kube/config` which is required to communicate with
kubernetes cluster or you can pass `kubeconfig` flag while running tests.
`

Functional tests are run by the `go test` command.

```console
go test ./e2e/ -timeout=20m -v -mod=vendor
```

To run specific tests, you can specify options

```console
go test ./e2e/ --test-cephfs=false --test-rbd=false --upgrade-testing=true
```

To run e2e for specific tests with `make`, use

```console
make run-e2e E2E_ARGS="--test-cephfs=false --test-rbd=true --upgrade-testing=false"
```

You can also invoke functional tests with `make` command

```console
make func-test TESTOPTIONS="-deploy-timeout=10 -timeout=30m -v"
```
