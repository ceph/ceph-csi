#!/bin/bash -e

#Based on ideas from https://github.com/rook/rook/blob/master/tests/scripts/minikube.sh

function wait_for_ssh() {
    local tries=100
    while ((tries > 0)); do
        if ${minikube} ssh echo connected &>/dev/null; then
            return 0
        fi
        tries=$((tries - 1))
        sleep 0.1
    done
    echo ERROR: ssh did not come up >&2
    exit 1
}

function copy_image_to_cluster() {
    local build_image=$1
    local final_image=$2
    validate_container_cmd
    if [ -z "$(${CONTAINER_CMD} images -q "${build_image}")" ]; then
        ${CONTAINER_CMD} pull "${build_image}"
    fi
    if [[ "${VM_DRIVER}" == "none" ]]; then
        ${CONTAINER_CMD} tag "${build_image}" "${final_image}"
        return
    fi

    # "minikube ssh" fails to read the image, so use standard ssh instead
    ${CONTAINER_CMD} save "${build_image}" | \
        ssh \
            -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
            -i "$(${minikube} ssh-key)" -l docker \
            "$(${minikube} ip)" docker image load
}

# parse the minikube version, return the digit passed as argument
# v1.11.0 -> minikube_version 1 -> 1
# v1.11.0 -> minikube_version 2 -> 11
# v1.11.0 -> minikube_version 3 -> 0
minikube_version() {
    echo "${MINIKUBE_VERSION}" | sed 's/^v//' | cut -d'.' -f"${1}"
}

# detect if there is a minikube executable available already. If there is none,
# fallback to using /usr/local/bin/minikube, as that is where
# install_minikube() will place it too.
function detect_minikube() {
    if type minikube >/dev/null 2>&1; then
        command -v minikube
        return
    fi
    # default if minikube is not available
    echo '/usr/local/bin/minikube'
}

# install minikube
function install_minikube() {
    if [[ "${MINIKUBE_VERSION}" == "latest" ]]; then
        local mku_version
        mku_version=$(${minikube} update-check 2> /dev/null | grep "LatestVersion" || true)
        if [[ -n "${mku_version}" ]]; then
            MINIKUBE_VERSION=$(echo "${mku_version}" | cut -d' ' -f2)
        fi
    fi

    if type "${minikube}" >/dev/null 2>&1; then
        local mk_version version
        read -ra mk_version <<<"$(${minikube} version)"
        version=${mk_version[2]}
        if [[ "${version}" == "${MINIKUBE_VERSION}" ]]; then
            echo "minikube already installed with ${version}"
            return
        fi
    fi

    echo "Installing minikube. Version: ${MINIKUBE_VERSION}"
    curl -Lo minikube https://storage.googleapis.com/minikube/releases/"${MINIKUBE_VERSION}"/minikube-linux-"${MINIKUBE_ARCH}" && chmod +x minikube && mv minikube /usr/local/bin/
}

function detect_kubectl() {
    if type kubectl >/dev/null 2>&1; then
        command -v kubectl
        return
    fi
    # default if kubectl is not available
    echo '/usr/local/bin/kubectl'
}

function install_kubectl() {
    if type "${kubectl}" >/dev/null 2>&1; then
        local kubectl_version
        kubectl_version=$(kubectl version --client --short | cut -d' ' -f3)
        if [[ "${kubectl_version}" == "${KUBE_VERSION}" ]]; then
            echo "kubectl already installed with ${kubectl_version}"
            return
        fi
    fi
    # Download kubectl, which is a requirement for using minikube.
    echo "Installing kubectl. Version: ${KUBE_VERSION}"
    curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/"${KUBE_VERSION}"/bin/linux/"${MINIKUBE_ARCH}"/kubectl && chmod +x kubectl && mv kubectl /usr/local/bin/
}

function validate_container_cmd() {
    local cmd="${CONTAINER_CMD##* }"
    if [[ "${cmd}" == "docker" ]] || [[ "${cmd}" == "podman" ]]; then
        if ! command -v "${cmd}" &> /dev/null; then
            echo "'${cmd}' not found"
            exit 1
        fi
    else
        echo "'CONTAINER_CMD' should be either docker or podman and not '${cmd}'"
        exit 1
    fi
}

function enable_psp() {
    echo "prepare minikube to support pod security policies"
    mkdir -p "$HOME"/.minikube/files/etc/kubernetes/addons
    DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
    cp "$DIR"/psp.yaml "$HOME"/.minikube/files/etc/kubernetes/addons/psp.yaml
}

# Storage providers and the default storage class is not needed for Ceph-CSI
# testing. In order to reduce resources and potential conflicts between storage
# plugins, disable them.
function disable_storage_addons() {
    ${minikube} addons disable default-storageclass 2>/dev/null || true
    ${minikube} addons disable storage-provisioner 2>/dev/null || true
}

# minikube has the Busybox losetup, and that does not work with raw-block PVCs.
# Copy the host losetup executable and hope it works.
#
# See https://github.com/kubernetes/minikube/issues/8284
function minikube_losetup() {
    # scp should not ask for any confirmation
    scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -i "$(${minikube} ssh-key)" /sbin/losetup docker@"$(${minikube} ip)":losetup
    # replace /sbin/losetup symlink with the executable
    ${minikube} ssh 'sudo sh -c "rm -f /sbin/losetup && cp ~docker/losetup /sbin"'
}

function minikube_supports_psp() {
    local MINIKUBE_MAJOR
    local MINIKUBE_MINOR
    local MINIKUBE_PATCH
    MINIKUBE_MAJOR=$(minikube_version 1)
    MINIKUBE_MINOR=$(minikube_version 2)
    MINIKUBE_PATCH=$(minikube_version 3)
    if [[ "${MINIKUBE_MAJOR}" -ge 1 ]] && [[ "${MINIKUBE_MINOR}" -ge 11 ]] && [[ "${MINIKUBE_PATCH}" -ge 1 ]] || [[ "${MINIKUBE_MAJOR}" -ge 1 ]] && [[ "${MINIKUBE_MINOR}" -ge 12 ]]; then
        return 1
    fi
    return 0
}

# configure minikube
MINIKUBE_ARCH=${MINIKUBE_ARCH:-"amd64"}
MINIKUBE_VERSION=${MINIKUBE_VERSION:-"latest"}
KUBE_VERSION=${KUBE_VERSION:-"latest"}
CONTAINER_CMD=${CONTAINER_CMD:-"docker"}
MEMORY=${MEMORY:-"4096"}
CPUS=${CPUS:-"$(nproc)"}
VM_DRIVER=${VM_DRIVER:-"virtualbox"}
#configure image repo
CEPHCSI_IMAGE_REPO=${CEPHCSI_IMAGE_REPO:-"quay.io/cephcsi"}
K8S_IMAGE_REPO=${K8S_IMAGE_REPO:-"k8s.gcr.io/sig-storage"}
DISK="sda1"
if [[ "${VM_DRIVER}" == "kvm2" ]]; then
    # use vda1 instead of sda1 when running with the libvirt driver
    DISK="vda1"
fi

#feature-gates for kube
K8S_FEATURE_GATES=${K8S_FEATURE_GATES:-"BlockVolume=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true,ExpandCSIVolumes=true"}

#extra-config for kube https://minikube.sigs.k8s.io/docs/reference/configuration/kubernetes/
EXTRA_CONFIG=${EXTRA_CONFIG:-"--extra-config=apiserver.enable-admission-plugins=PodSecurityPolicy"}

# kubelet.resolv-conf needs to point to a file, not a symlink
# the default minikube VM has /etc/resolv.conf -> /run/systemd/resolve/resolv.conf
RESOLV_CONF='/run/systemd/resolve/resolv.conf'
if [[ "${VM_DRIVER}" == "none" ]] && [[ ! -e "${RESOLV_CONF}" ]]; then
	# in case /run/systemd/resolve/resolv.conf does not exist, use the
	# standard /etc/resolv.conf (with symlink resolved)
	RESOLV_CONF="$(readlink -f /etc/resolv.conf)"
fi
# TODO: this might overload --extra-config=kubelet.resolv-conf in case the
# caller did set EXTRA_CONFIG in the environment
EXTRA_CONFIG="${EXTRA_CONFIG} --extra-config=kubelet.resolv-conf=${RESOLV_CONF}"

#extra Rook configuration
ROOK_BLOCK_POOL_NAME=${ROOK_BLOCK_POOL_NAME:-"newrbdpool"}

if [[ "${KUBE_VERSION}" == "latest" ]]; then
    # update the version string from latest with the real version
    KUBE_VERSION=$(curl -L https://storage.googleapis.com/kubernetes-release/release/stable.txt 2> /dev/null)
fi

minikube="$(detect_minikube)"
kubectl="$(detect_kubectl)"

case "${1:-}" in
up)
    install_minikube
    #if driver  is 'none' install kubectl with KUBE_VERSION
    if [[ "${VM_DRIVER}" == "none" ]]; then
        mkdir -p "$HOME"/.kube "$HOME"/.minikube
        install_kubectl
    fi

    disable_storage_addons

    echo "starting minikube with kubeadm bootstrapper"
    if minikube_supports_psp; then
        enable_psp
        # shellcheck disable=SC2086
        ${minikube} start --force --memory="${MEMORY}" --cpus="${CPUS}" -b kubeadm --kubernetes-version="${KUBE_VERSION}" --driver="${VM_DRIVER}" --feature-gates="${K8S_FEATURE_GATES}" ${EXTRA_CONFIG}
    else
        # This is a workaround to fix psp issues in minikube >1.6.2 and <1.11.0
        # shellcheck disable=SC2086
        ${minikube} start --force --memory="${MEMORY}" --cpus="${CPUS}" -b kubeadm --kubernetes-version="${KUBE_VERSION}" --driver="${VM_DRIVER}" --feature-gates="${K8S_FEATURE_GATES}"
        DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
        ${minikube} kubectl -- apply -f "$DIR"/psp.yaml
        ${minikube} stop
        # shellcheck disable=SC2086
        ${minikube} start --force --memory="${MEMORY}" --cpus="${CPUS}" -b kubeadm --kubernetes-version="${KUBE_VERSION}" --driver="${VM_DRIVER}" --feature-gates="${K8S_FEATURE_GATES}" ${EXTRA_CONFIG}
    fi

    # create a link so the default dataDirHostPath will work for this
    # environment
    if [[ "${VM_DRIVER}" != "none" ]]; then
        wait_for_ssh
        # shellcheck disable=SC2086
        ${minikube} ssh "sudo mkdir -p /mnt/${DISK}/var/lib/rook;sudo ln -s /mnt/${DISK}/var/lib/rook /var/lib/rook"
        minikube_losetup
    fi
    ${minikube} kubectl -- cluster-info
    ;;
down)
    ${minikube} stop
    ;;
ssh)
    echo "connecting to minikube"
    ${minikube} ssh
    ;;
deploy-rook)
    echo "deploy rook"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/rook.sh deploy
    ;;
install-snapshotter)
    echo "install snapshot controller"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/install-snapshot.sh install
    ;;
create-block-pool)
    echo "creating a block pool named $ROOK_BLOCK_POOL_NAME"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/rook.sh create-block-pool
    ;;
delete-block-pool)
    echo "deleting block pool named $ROOK_BLOCK_POOL_NAME"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/rook.sh delete-block-pool
    ;;
cleanup-snapshotter)
    echo "cleanup snapshot controller"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/install-snapshot.sh cleanup
    ;;
teardown-rook)
    echo "teardown rook"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/rook.sh teardown

    # delete rook data for minikube
    ${minikube} ssh "sudo rm -rf /mnt/${DISK}/var/lib/rook; sudo rm -rf /var/lib/rook"
    ${minikube} ssh "sudo mkdir -p /mnt/${DISK}/var/lib/rook; sudo ln -s /mnt/${DISK}/var/lib/rook /var/lib/rook"
    ;;
cephcsi)
    echo "copying the cephcsi image"
    copy_image_to_cluster "${CEPHCSI_IMAGE_REPO}"/cephcsi:canary "${CEPHCSI_IMAGE_REPO}"/cephcsi:canary
    ;;
k8s-sidecar)
    echo "copying the kubernetes sidecar images"
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-attacher:v3.0.2 "${K8S_IMAGE_REPO}"/csi-attacher:v3.0.2
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-snapshotter:v3.0.2 $"${K8S_IMAGE_REPO}"/csi-snapshotter:v3.0.2
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-provisioner:v2.0.4 "${K8S_IMAGE_REPO}"/csi-provisioner:v2.0.4
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-node-driver-registrar:v2.0.1 "${K8S_IMAGE_REPO}"/csi-node-driver-registrar:v2.0.1
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-resizer:v1.0.1 "${K8S_IMAGE_REPO}"/csi-resizer:v1.0.1
    ;;
clean)
    ${minikube} delete
    ;;
*)
    echo " $0 [command]
Available Commands:
  up                   Starts a local kubernetes cluster and prepare disk for rook
  down                 Stops a running local kubernetes cluster
  clean                Deletes a local kubernetes cluster
  ssh                  Log into or run a command on a minikube machine with SSH
  deploy-rook          Deploy rook to minikube
  install-snapshotter  Install snapshot controller
  create-block-pool    Creates a rook block pool (named $ROOK_BLOCK_POOL_NAME)
  delete-block-pool    Deletes a rook block pool (named $ROOK_BLOCK_POOL_NAME)
  cleanup-snapshotter  Cleanup snapshot controller
  teardown-rook        Teardown rook from minikube
  cephcsi              Copy built docker images to kubernetes cluster
  k8s-sidecar          Copy kubernetes sidecar docker images to kubernetes cluster
" >&2
    ;;
esac
