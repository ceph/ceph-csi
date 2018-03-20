#!/bin/sh

kubectl logs $(kubectl get pods -l app=csi-cephfsplugin -o=name | head -n 1) -c csi-cephfsplugin
