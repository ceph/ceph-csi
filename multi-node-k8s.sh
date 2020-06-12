#!/bin/sh

# TODO: disable debugging
set -x

# get latest stable version
# K8S_VERSION="$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)"
# kubernetes 1.18.x needs changes to the e2e tests
K8S_VERSION="v1.17.5"

# configure cluster parameters
WORKERS=1
WORKER_MEM=4
WORKER_CPU=2
DISKS=2
DISK_SIZE=25
MASTER_MEM=4
MASTER_CPU=2
K8S_NETWORK="calico"
POD_NET_CIDR=192.168.123.0/24
# BOX_OS_IMAGE="centos/8"
BOX_OS="centos8"
OS_DISK_SIZE_GB=25

# enable additional sources for yum
# (epel for ansible and golang)
yum -y install epel-release

# Install additional packages
# yum -y install \
# 	qemu-kvm \
# 	virt-install \
# 	make \
# 	libvirt-client \
# 	# qemu-kvm-tools \
# 	qemu-img \
# 	libvirt \
# 	ansible
dnf install -y qemu-kvm qemu-img libvirt virt-install libvirt-client ansible make rsync
dnf module -y install virt
lsmod | grep kvm

# install kubectl
if [ ! -x /usr/bin/kubectl ]; then
	curl -LO https://storage.googleapis.com/kubernetes-release/release/"${K8S_VERSION}"/bin/linux/amd64/kubectl
	mv kubectl /usr/bin/kubectl
	chmod +x /usr/bin/kubectl
fi

# Vagrant needs libvirtd running
systemctl start libvirtd.service
systemctl enable libvirtd.service
systemctl status libvirtd.service

# Log the virsh capabilites so that we know the
# environment in case something goes wrong.
virsh capabilities

# TODO: this is not the right way to install vagrant on CentOS, but SCL only
# provides vagrant 1.x and we need >= 2.2
if ! command -v vagrant; then
	yum -y install https://releases.hashicorp.com/vagrant/2.2.7/vagrant_2.2.7_x86_64.rpm
	yum -y install gcc libvirt-devel
	vagrant plugin install vagrant-libvirt
fi

# yum install -y https://download.docker.com/linux/centos/7/x86_64/edge/Packages/docker-ce-18.05.0.ce-3.el7.centos.x86_64.rpm
# yum install docker-ce
# containerdFile=https://download.docker.com/linux/centos/7/x86_64/stable/Packages/containerd.io-1.2.13-3.2.el7.x86_64.rpm
# dnf install "${containerdFile}" -y

# vagrant plugin install vagrant-libvirt

# setup the kubernes cluster
git clone https://github.com/galexrt/k8s-vagrant-multi-node
cd k8s-vagrant-multi-node || exit

make preflight
make up VAGRANT_DEFAULT_PROVIDER=libvirt KUBERNETES_VERSION="${K8S_VERSION}" MASTER_CPUS=${MASTER_CPU} NODE_CPUS=${WORKER_CPU} MASTER_MEMORY_SIZE_GB=${MASTER_MEM} NODE_COUNT=${WORKERS} NODE_MEMORY_SIZE_GB=${WORKER_MEM} DISK_COUNT=${DISKS} DISK_SIZE_GB=${DISK_SIZE} KUBE_NETWORK=${K8S_NETWORK} BOX_OS="${BOX_OS}" POD_NW_CIDR=${POD_NET_CIDR}

# resize OS disk
make stop-master
virsh vol-resize k8s-vagrant-multi-node_master.img ${OS_DISK_SIZE_GB}G --pool=default
make start-master VAGRANT_DEFAULT_PROVIDER=libvirt
make VAGRANT_DEFAULT_PROVIDER=libvirt ssh-master <<< 'sudo sh -c "dnf -y --disablerepo=kubernetes install cloud-utils-growpart && growpart /dev/sda 1 && xfs_growfs /"'

for NODE in $(seq 1 ${WORKERS}); do
	make NODE_COUNT=${WORKERS} stop-node-${NODE}
	virsh vol-resize k8s-vagrant-multi-node_node${NODE}.img ${OS_DISK_SIZE_GB}G --pool=default
	make VAGRANT_DEFAULT_PROVIDER=libvirt NODE_COUNT=${WORKERS} ssh-node-${NODE} <<< 'sudo sh -c "dnf -y --disablerepo=kubernetes install cloud-utils-growpart && growpart /dev/sda 1 && xfs_growfs /"'
	make start-node-${NODE} VAGRANT_DEFAULT_PROVIDER=libvirt NODE_COUNT=${WORKERS}
done

make status NODE_COUNT=${WORKERS}
# BOX_IMAGE="${BOX_OS_IMAGE}"
