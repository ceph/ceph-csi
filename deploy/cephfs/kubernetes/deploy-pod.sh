#!/bin/sh

kubectl create -f ./pvc.yaml
kubectl create -f ./pod.yaml
