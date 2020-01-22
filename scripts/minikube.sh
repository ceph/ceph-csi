#!/bin/bash -e

#Based on ideas from https://github.com/rook/rook/blob/master/tests/scripts/minikube.sh

function wait_for_ssh() {
    local tries=100
    while ((tries > 0)); do
        if minikube ssh echo connected &>/dev/null; then
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
    if [ -z "$(docker images -q "${build_image}")" ]; then
        docker pull "${build_image}"
    fi
    if [[ "${VM_DRIVER}" == "none" ]]; then
        docker tag "${build_image}" "${final_image}"
        return
    fi
    docker save "${build_image}" | (eval "$(minikube docker-env --shell bash)" && docker load && docker tag "${build_image}" "${final_image}")
}

# install minikube
function install_minikube() {
    if type minikube >/dev/null 2>&1; then
        local version
        version=$(minikube version)
        read -ra version <<<"${version}"
        version=${version[2]}
        if [[ "${version}" != "${MINIKUBE_VERSION}" ]]; then
            echo "installed minikube version ${version} is not matching requested version ${MINIKUBE_VERSION}"
            exit 1
        fi
        echo "minikube already installed with ${version}"
        return
    fi

    echo "Installing minikube. Version: ${MINIKUBE_VERSION}"
    curl -Lo minikube https://storage.googleapis.com/minikube/releases/"${MINIKUBE_VERSION}"/minikube-linux-"${MINIKUBE_ARCH}" && chmod +x minikube && mv minikube /usr/local/bin/
}

function install_kubectl() {
    # Download kubectl, which is a requirement for using minikube.
    echo "Installing kubectl. Version: ${KUBE_VERSION}"
    curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/"${KUBE_VERSION}"/bin/linux/"${MINIKUBE_ARCH}"/kubectl && chmod +x kubectl && mv kubectl /usr/local/bin/
}

function enable_psp() {
    echo "prepare minikube to support pod security policies"
    mkdir -p "$HOME"/.minikube/files/etc/kubernetes/addons
    DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
	  cp "$DIR"/psp.yaml "$HOME"/.minikube/files/etc/kubernetes/addons/psp.yaml
}

# configure minikube
MINIKUBE_ARCH=${MINIKUBE_ARCH:-"amd64"}
MINIKUBE_VERSION=${MINIKUBE_VERSION:-"latest"}
KUBE_VERSION=${KUBE_VERSION:-"v1.14.10"}
MEMORY=${MEMORY:-"3000"}
VM_DRIVER=${VM_DRIVER:-"virtualbox"}
#configure image repo
CEPHCSI_IMAGE_REPO=${CEPHCSI_IMAGE_REPO:-"quay.io/cephcsi"}
K8S_IMAGE_REPO=${K8S_IMAGE_REPO:-"quay.io/k8scsi"}
DISK="sda1"
if [[ "${VM_DRIVER}" == "kvm2" ]]; then
    # use vda1 instead of sda1 when running with the libvirt driver
    DISK="vda1"
fi

#feature-gates for kube
K8S_FEATURE_GATES=${K8S_FEATURE_GATES:-"BlockVolume=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true,ExpandCSIVolumes=true"}

#extra-config for kube https://minikube.sigs.k8s.io/docs/reference/configuration/kubernetes/
EXTRA_CONFIG=${EXTRA_CONFIG:-"--extra-config=apiserver.enable-admission-plugins=PodSecurityPolicy"}

case "${1:-}" in
up)
    install_minikube
    #if driver  is 'none' install kubectl with KUBE_VERSION
    if [[ "${VM_DRIVER}" == "none" ]]; then
        mkdir -p "$HOME"/.kube "$HOME"/.minikube
        install_kubectl
    fi

    enable_psp

    echo "starting minikube with kubeadm bootstrapper"
    # shellcheck disable=SC2086
    minikube start --memory="${MEMORY}" -b kubeadm --kubernetes-version="${KUBE_VERSION}" --vm-driver="${VM_DRIVER}" --feature-gates="${K8S_FEATURE_GATES}" ${EXTRA_CONFIG}

    # create a link so the default dataDirHostPath will work for this
    # environment
    if [[ "${VM_DRIVER}" != "none" ]]; then
        wait_for_ssh
        # shellcheck disable=SC2086
        minikube ssh "sudo mkdir -p /mnt/${DISK}/${PWD}; sudo mkdir -p $(dirname $PWD); sudo ln -s /mnt/${DISK}/${PWD} $(dirname $PWD)/"
        minikube ssh "sudo mkdir -p /mnt/${DISK}/var/lib/rook;sudo ln -s /mnt/${DISK}/var/lib/rook /var/lib/rook"
    fi
    kubectl cluster-info
    ;;
down)
    minikube stop
    ;;
ssh)
    echo "connecting to minikube"
    minikube ssh
    ;;
deploy-rook)
    echo "deploy rook"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/rook.sh deploy
    ;;
teardown-rook)
    echo "teardown rook"
    DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
    "$DIR"/rook.sh teardown

    # delete rook data for minikube
    minikube ssh "sudo rm -rf /mnt/${DISK}/var/lib/rook; sudo rm -rf /var/lib/rook"
    minikube ssh "sudo mkdir -p /mnt/${DISK}/var/lib/rook; sudo ln -s /mnt/${DISK}/var/lib/rook /var/lib/rook"
    ;;
cephcsi)
    echo "copying the cephcsi image"
    copy_image_to_cluster "${CEPHCSI_IMAGE_REPO}"/cephcsi:v2.0-canary "${CEPHCSI_IMAGE_REPO}"/cephcsi:v2.0-canary
    ;;
k8s-sidecar)
    echo "copying the kubernetes sidecar images"
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-attacher:v1.2.1 "${K8S_IMAGE_REPO}"/csi-attacher:v1.2.1
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-attacher:v2.1.0 "${K8S_IMAGE_REPO}"/csi-attacher:v2.1.0
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-snapshotter:v1.2.2 $"${K8S_IMAGE_REPO}"/csi-snapshotter:v1.2.2
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-provisioner:v1.4.0 "${K8S_IMAGE_REPO}"/csi-provisioner:v1.4.0
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-node-driver-registrar:v1.2.0 "${K8S_IMAGE_REPO}"/csi-node-driver-registrar:v1.2.0
    copy_image_to_cluster "${K8S_IMAGE_REPO}"/csi-resizer:v0.4.0 "${K8S_IMAGE_REPO}"/csi-resizer:v0.4.0
    ;;
clean)
    minikube delete
    ;;
*)
    echo " $0 [command]
Available Commands:
  up             Starts a local kubernetes cluster and prepare disk for rook
  down           Stops a running local kubernetes cluster
  clean          Deletes a local kubernetes cluster
  ssh            Log into or run a command on a minikube machine with SSH
  deploy-rook    Deploy rook to minikube
  teardown-rook  Teardown a rook from minikube
  cephcsi        copy built docker images to kubernetes cluster
  k8s-sidecar    copy kubernetes sidecar docker images to kubernetes cluster
" >&2
    ;;
esac
