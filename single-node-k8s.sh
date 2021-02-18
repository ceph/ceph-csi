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
    export MEMORY="12288"
    export CEPH_CSI_RUN_ALL_TESTS=true
    # downloading rook images is sometimes slow, extend timeout to 15 minutes
    export ROOK_VERSION=${ROOK_VERSION:-'v1.3.9'}
    export ROOK_DEPLOY_TIMEOUT=900
    export ROOK_CEPH_CLUSTER_IMAGE="${ROOK_CEPH_CLUSTER_IMAGE}"
    # use podman for minikube.sh, Docker is not installed on the host
    export CONTAINER_CMD='podman'

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
    MINIKUBE_VERSION="${MINIKUBE_VERSION}" KUBE_VERSION="${k8s_version}" ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh up

    # copy kubectl from minikube to /usr/bin
    cp ~/.minikube/cache/linux/"${k8s_version}"/kubectl /usr/bin/

    # add disks to the minikube VM
    if [ ! -d /opt/minikube/images ]; then
        mkdir -p /opt/minikube/images
        virsh pool-create-as --name minikube --type dir --target /opt/minikube/images
    fi
    virsh vol-create-as --pool minikube --name osd-0 --capacity 32G
    virsh vol-create-as --pool minikube --name osd-1 --capacity 32G
    virsh vol-create-as --pool minikube --name osd-2 --capacity 32G
    virsh attach-disk --domain minikube --source /opt/minikube/images/osd-0 --target vdb
    virsh attach-disk --domain minikube --source /opt/minikube/images/osd-1 --target vdc
    virsh attach-disk --domain minikube --source /opt/minikube/images/osd-2 --target vdd
    # rescan for newly attached virtio devices
    minikube ssh 'echo 1 | sudo tee /sys/bus/pci/rescan > /dev/null ; dmesg | grep virtio_blk'
}

function deploy_rook()
{
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh deploy-rook
    ${GOPATH}/src/github.com/ceph/ceph-csi/scripts/minikube.sh create-block-pool
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

deploy_rook

# running e2e.test requires librados and librbd
dnf -y install librados2 librbd1

# now it is time to run the e2e tests!
