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
    echo "###"
    echo "### going to execute: ${*}"
    echo "###"
    "${@}"
    echo "###"
    echo "### execution finished: ${*}"
    echo "###"
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
log df -h
log minikube_ssh df -h
