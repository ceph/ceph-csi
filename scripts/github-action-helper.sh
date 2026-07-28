#!/usr/bin/env bash

set -xeEo pipefail

# Find a non-boot block device for Rook OSD.
# Adapted from rook/ceph-csi-operator helpers.
find_extra_block_dev() {
  sudo lsblk >&2
  local exclude="loop"
  local boot_dev
  boot_dev="$(sudo lsblk --noheading --list --output MOUNTPOINT,PKNAME | grep boot | awk '{print $2}' | sort -u | tr '\n' '|' | sed 's/|$//')"
  if [ -n "${boot_dev}" ]; then
    exclude+="|${boot_dev}"
  fi
  local root_dev
  root_dev="$(sudo lsblk --noheading --list --output MOUNTPOINT,PKNAME | awk '$1=="/" {print $2}' | sort -u | tr '\n' '|' | sed 's/|$//')"
  if [ -n "${root_dev}" ]; then
    exclude+="|${root_dev}"
  fi
  local extra_dev
  extra_dev="$(sudo lsblk --noheading --list --nodeps --output KNAME,TYPE | awk '$2=="disk" {print $1}' | grep -Ev "(${exclude})" | head -1)"
  if [ -z "${extra_dev}" ]; then
    echo "No extra block device found" >&2
    echo "Creating iSCSI target disk" >&2
    sudo apt-get install -y -q targetcli-fb open-iscsi >&2
    truncate -s 20G ~/iscsi-disk.img
    local tgt="iqn.2026-07.target.local:disk1"
    local ini="iqn.2026-07.initiator.local"
    sudo targetcli /backstores/fileio \
      create disk1 ~/iscsi-disk.img 20G >&2
    sudo targetcli /iscsi create "${tgt}" >&2
    sudo targetcli "/iscsi/${tgt}/tpg1/luns" \
      create /backstores/fileio/disk1 >&2
    echo "InitiatorName=${ini}" \
      | sudo tee /etc/iscsi/initiatorname.iscsi \
      >/dev/null
    sudo targetcli "/iscsi/${tgt}/tpg1/acls" \
      create "${ini}" >&2
    sudo iscsiadm -m discovery \
      -t sendtargets -p 127.0.0.1 >&2
    sudo iscsiadm -m node --login >&2
    sleep 3
    extra_dev="$(sudo lsblk --noheading --list \
      --nodeps --output KNAME,TYPE \
      | awk '$2=="disk" {print $1}' \
      | grep -Ev "(${exclude})" | head -1)"
    echo "iSCSI device: ${extra_dev}" >&2
  fi
  echo "${extra_dev}"
}

prepare_disk() {
  : "${BLOCK:=$(find_extra_block_dev)}"
  if [ -z "${BLOCK}" ] || [ ! -b "/dev/${BLOCK}" ]; then
    echo "BLOCK device /dev/${BLOCK} not found" >&2
    exit 1
  fi
  sudo swapoff --all --verbose || true
  sudo lsblk
  if mountpoint -q /mnt 2>/dev/null; then
    sudo umount /mnt
  fi
  sudo wipefs --all --force "/dev/${BLOCK}"
  sudo sgdisk --zap-all "/dev/${BLOCK}"
  sudo dd if=/dev/zero of="/dev/${BLOCK}" \
    bs=1M count=100 oflag=direct
  sudo partprobe "/dev/${BLOCK}" || true
  sudo lsblk
}

install_minikube_prereqs() {
  sudo apt-get update -qq
  sudo apt-get install -y -qq conntrack socat
  # cri-dockerd
  local cridv="0.3.17"
  local gh="https://github.com/Mirantis/cri-dockerd"
  local tgz="cri-dockerd-${cridv}.amd64.tgz"
  curl -sfLO "${gh}/releases/download/v${cridv}/${tgz}"
  tar -xzf "${tgz}"
  sudo install -m 0755 cri-dockerd/cri-dockerd \
    /usr/local/bin/cri-dockerd
  rm -rf cri-dockerd "${tgz}"
  local u
  for u in cri-docker.service cri-docker.socket; do
    curl -sfLO \
      "${gh}/raw/v${cridv}/packaging/systemd/${u}"
    sudo install -m 0644 "${u}" /etc/systemd/system/
  done
  sudo sed -i \
    's|/usr/bin/cri-dockerd|/usr/local/bin/cri-dockerd|' \
    /etc/systemd/system/cri-docker.service
  sudo systemctl daemon-reload
  sudo systemctl enable --now cri-docker.socket
  # crictl
  local crictlv="v1.35.0"
  local crictl_base="https://github.com"
  crictl_base+="/kubernetes-sigs/cri-tools"
  local crictl_tar="crictl-${crictlv}-linux-amd64.tar.gz"
  curl -sLO \
    "${crictl_base}/releases/download/${crictlv}/${crictl_tar}"
  sudo tar -xzf "${crictl_tar}" -C /usr/local/bin
  rm -f "${crictl_tar}"
  # CNI plugins
  local cniv="v1.6.2"
  local cni_base="https://github.com"
  cni_base+="/containernetworking/plugins"
  local cni_tar="cni-plugins-linux-amd64-${cniv}.tgz"
  sudo mkdir -p /opt/cni/bin
  curl -sLO \
    "${cni_base}/releases/download/${cniv}/${cni_tar}"
  sudo tar -xzf "${cni_tar}" -C /opt/cni/bin
  rm -f "${cni_tar}"
}

collect_logs() {
  LOG_DIR="/tmp/acceptance-e2e-logs"
  mkdir -p "${LOG_DIR}"
  # Rook-Ceph namespace
  for ns in rook-ceph default ceph-csi-operator-system; do
    kubectl -n "${ns}" get pods -o wide > "${LOG_DIR}/${ns}-pods.txt" 2>&1 || true
    kubectl -n "${ns}" get events --sort-by='.lastTimestamp' > "${LOG_DIR}/${ns}-events.txt" 2>&1 || true
    for pod in $(kubectl -n "${ns}" get pods -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
      kubectl -n "${ns}" logs "${pod}" --all-containers --tail=200 > "${LOG_DIR}/${ns}-${pod}.log" 2>&1 || true
    done
  done
  # PVC and PV state
  kubectl get pvc --all-namespaces > "${LOG_DIR}/pvcs.txt" 2>&1 || true
  kubectl get pv > "${LOG_DIR}/pvs.txt" 2>&1 || true
  # Ceph status from toolbox
  kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status > "${LOG_DIR}/ceph-status.txt" 2>&1 || true
  kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd tree > "${LOG_DIR}/ceph-osd-tree.txt" 2>&1 || true
  echo "Logs collected in ${LOG_DIR}"
}

########
# MAIN #
########

FUNCTION="$1"
shift
if ! "${FUNCTION}" "$@"; then
  echo "Call to ${FUNCTION} was not successful" >&2
  exit 1
fi
