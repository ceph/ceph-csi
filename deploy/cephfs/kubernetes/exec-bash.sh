#!/bin/sh

kubectl exec -it $(kubectl get pods -l app=csi-cephfsplugin -o=name | head -n 1 | cut -f2 -d"/") -c csi-cephfsplugin bash
