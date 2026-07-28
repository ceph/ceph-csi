#!/usr/bin/env bash

set -xeEo pipefail

#############
# VARIABLES #
#############
: "${FUNCTION:=${1}}"

# Find a non-boot block device for Rook OSD.
# Adapted from rook/ceph-csi-operator helpers.
find_extra_block_dev() {
  sudo lsblk >&2
  boot_dev="$(sudo lsblk --noheading --list --output MOUNTPOINT,PKNAME | grep boot | awk '{print $2}' | sort -u)"
  extra_dev="$(sudo lsblk --noheading --list --nodeps --output KNAME | grep -Ev "(loop|${boot_dev})" | head -1)"
  if [ -z "${extra_dev}" ]; then
    echo "No extra block device found, creating a loopback device" >/dev/stderr
    sudo fallocate -l 20G /var/lib/rook-disk.img
    extra_dev="$(sudo losetup --show -f /var/lib/rook-disk.img)"
    extra_dev="$(basename "${extra_dev}")"
  fi
  echo "${extra_dev}"
}

: "${BLOCK:=$(find_extra_block_dev)}"

prepare_disk() {
  sudo swapoff --all --verbose || true
  sudo lsblk
  if mountpoint -q /mnt 2>/dev/null; then
    sudo umount /mnt
    BLOCK_DATA_PART="/dev/${BLOCK}1"
    if [ -b "${BLOCK_DATA_PART}" ]; then
      sudo wipefs --all --force "${BLOCK_DATA_PART}"
    fi
  fi
}

collect_logs() {
  LOG_DIR="/tmp/tier1-e2e-logs"
  mkdir -p "${LOG_DIR}"
  # Rook-Ceph namespace
  for ns in rook-ceph default; do
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
