#!/bin/bash
#
# Run this script to gather details about the environment where the CI job is
# running. This can be helpful to identify issues why minikube failed to
# deploy, or tests encounter problems while running.
#

function minikube_ssh() {
    ssh \
        -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
        -l docker -i "$(minikube ssh-key)" \
        "$(minikube ip)" "${*}"
}

function log() {
    echo "###" >/dev/stderr
    echo "### going to execute: ${*}" >/dev/stderr
    echo "###" >/dev/stderr
    "${@}"
    echo "###" >/dev/stderr
    echo "### execution finished: ${*}" >/dev/stderr
    echo "###" >/dev/stderr
}

# get the status of the VM in libvirt
log virsh list

# status of the minikube Kubernetes cluster
log minikube status
log minikube logs

# get the status of processes in the VM
log minikube_ssh top -b -c -n1 -w

# get the logs from the VM
log minikube_ssh journalctl --boot

# filesystem status for host and VM
log df -hT
log minikube_ssh df -hT

# fetch all logs from /var/lib/rook in the VM and write them to stdout
log minikube_ssh sudo tar c /var/lib/rook | tar xvO

# gets status of the Rook deployment
log kubectl -n rook-ceph get pods
log kubectl -n rook-ceph describe pods
log kubectl -n rook-ceph describe CephCluster
log kubectl -n rook-ceph describe CephBlockPool
log kubectl -n rook-ceph describe CephFilesystem

# run "ceph -s" in the toolbox
log kubectl -n rook-ceph exec \
    "$(kubectl -n rook-ceph get pod -l app=rook-ceph-tools -o jsonpath='{.items[0].metadata.name}')" \
    ceph -s
