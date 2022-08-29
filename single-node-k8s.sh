#!/bin/bash
#
# Start minukube with the selected k8s version and deploy rook.
#
# This script uses helper scripts from the main ceph-csi branch.
#

# fail when a command returns an error
set -e -o pipefail

ARGUMENT_LIST=(
    "k8s-version"
)

opts=$(getopt \
    --longoptions "$(printf "%s:," "${ARGUMENT_LIST[@]}")help" \
    --name "$(basename "${0}")" \
    --options "" \
    -- "$@"
)
ret=$?

if [ ${ret} -ne 0 ]
then
    echo "Try '--help' for more information."
    exit 1
fi

eval set -- "${opts}"

while true; do
    case "${1}" in
    --help)
        shift
        echo "Options:"
        echo "--help|-h                 specify the flags"
        echo "--k8s-version             specify the kubernetes version"

        echo " "
        echo "Sample Usage:"
        echo "./single-node-k8s.sh --k8s-version=v1.18.3"
        exit 0
        ;;
    --k8s-version)
        shift
        k8s_version=${1}
        ;;
    --)
        shift
        break
        ;;
    esac
    shift
done

test -n "${k8s_version}" || { echo "k8s_version is not set"; exit 1;}

function set_env() {
    export GOPATH="/opt/build/go"
    # build.env is not part of the ci/centos branch, so shellcheck can not find
    # it and will cause a failure if not annotated with /dev/null.
    # shellcheck source=/dev/null
    source "${GOPATH}"/src/github.com/ceph/ceph-csi/build.env
    CSI_IMAGE_VERSION=${CSI_IMAGE_VERSION:-"canary"}
    export GO111MODULE="on"
    export TEST_COVERAGE="stdout"
    export VM_DRIVER="kvm2"
    export MEMORY="14336"
    export NUM_DISKS="3"
    export DISK_SIZE="32gb"
    export CEPH_CSI_RUN_ALL_TESTS=true
    # downloading rook images is sometimes slow, extend timeout to 15 minutes
    export ROOK_VERSION=${ROOK_VERSION:-'v1.3.9'}
    export ROOK_DEPLOY_TIMEOUT=900
    export ROOK_CEPH_CLUSTER_IMAGE="${ROOK_CEPH_CLUSTER_IMAGE}"
    # use podman for minikube.sh, Docker is not installed on the host
    export CONTAINER_CMD='podman'

    export CSI_ATTACHER_VERSION=${CSI_ATTACHER_VERSION:-"v3.2.1"}
    export CSI_SNAPSHOTTER_VERSION=${CSI_SNAPSHOTTER_VERSION:-"v4.1.1"}
    export CSI_PROVISIONER_VERSION=${CSI_PROVISIONER_VERSION:-"v2.2.2"}
    export CSI_RESIZER_VERSION=${CSI_RESIZER_VERSION:-"v1.2.0"}
    export CSI_NODE_DRIVER_REGISTRAR_VERSION=${CSI_NODE_DRIVER_REGISTRAR_VERSION:-"v2.2.0"}

    # script/minikube.sh installs under /usr/local/bin
    export PATH=$PATH:/usr/local/bin
}

function install_minikube()
{
    dnf -y groupinstall 'Virtualization Host'
    systemctl enable --now libvirtd
    # Warning about "No ACPI IVRS table found", not critical
    virt-host-validate || true

    # minikube needs socat
    dnf -y install socat

    # deploy minikube
    MINIKUBE_VERSION="${MINIKUBE_VERSION}" MINIKUBE_ISO_URL="${MINIKUBE_ISO_URL}" KUBE_VERSION="${k8s_version}" ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh up

    # copy kubectl from minikube to /usr/bin
    if [ -x ~/.minikube/cache/linux/"${k8s_version}"/kubectl ]
    then
        cp ~/.minikube/cache/linux/"${k8s_version}"/kubectl /usr/bin/
    else
        # minikube 1.25.2 adds the GOARCH to the path ("amd64" in our CI)
        cp ~/.minikube/cache/linux/amd64/"${k8s_version}"/kubectl /usr/bin/
    fi

    # scan for extra disks
    minikube ssh 'echo 1 | sudo tee /sys/bus/pci/rescan > /dev/null ; dmesg | grep virtio_blk'
}

function deploy_rook()
{
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh deploy-rook
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh create-block-pool
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh create-block-ec-pool
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh k8s-sidecar

    # TODO: only needed for k8s 1.17.0 and newer
    # delete snapshot CRD created by ceph-csi in rook
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/install-snapshot.sh delete-crd || true
    # install snapshot controller
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/install-snapshot.sh install
}

# Set environment variables
set_env

# prepare minikube environment
install_minikube

./podman2minikube.sh "quay.io/cephcsi/cephcsi:${CSI_IMAGE_VERSION}"

# incase rook/ceph is available on the local system, push it into the VM
if podman inspect "rook/ceph:${ROOK_VERSION}" > /dev/null
then
    ./podman2minikube.sh "rook/ceph:${ROOK_VERSION}"
fi

# Rook also uses ceph/ceph:v15 (build.env:BASE_IMAGE), so push it into the VM
if [ -n "${BASE_IMAGE}" ] && podman inspect "${BASE_IMAGE}" > /dev/null
then
    ./podman2minikube.sh "${BASE_IMAGE}"
fi

# Rook also uses ceph/ceph:v15 (build.env:ROOK_CEPH_CLUSTER_IMAGE), so push it into the VM
if [ -n "${ROOK_CEPH_CLUSTER_IMAGE}" ] && podman inspect "${ROOK_CEPH_CLUSTER_IMAGE}" > /dev/null
then
    ./podman2minikube.sh "${ROOK_CEPH_CLUSTER_IMAGE}"
fi

deploy_rook

# running e2e.test requires librados and librbd
dnf -y install librados2 librbd1

# now it is time to run the e2e tests!
