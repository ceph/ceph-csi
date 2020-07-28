#!/bin/bash

deployment_base="${1}"

if [[ -z $deployment_base ]]; then
	deployment_base="../../deploy/rbd/kubernetes/with-rbac.yaml"
fi

if [ -e $deployment_base ]; then
  kubectl delete -f "$deployment_base"
else
  echo "File or directory does not exist: $deployment_base"
  exit 1
fi
