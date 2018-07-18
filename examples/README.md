## How to test RBD and CephFS plugins with Kubernetes 1.11

Both `rbd` and `cephfs` directories contain `plugin-deploy.sh` and `plugin-teardown.sh` helper scripts. You can use those to help you deploy/tear down RBACs, sidecar containers and the plugin in one go. By default, they look for the YAML manifests in `../../deploy/{rbd,cephfs}/kubernetes`. You can override this path by running `$ ./plugin-deploy.sh /path/to/my/manifests`.

Once the plugin is successfuly deployed, you'll need to customize `storageclass.yaml` and `secret.yaml` manifests to reflect your Ceph cluster setup. Please consult the documentation for info about available parameters.

After configuring the secrets, monitors, etc. you can deploy a testing Pod mounting a RBD image / CephFS volume:
```bash
$ kubectl create -f secret.yaml
$ kubectl create -f storageclass.yaml
$ kubectl create -f pvc.yaml
$ kubectl create -f pod.yaml
```

Other helper scripts:
* `logs.sh` output of the plugin
* `exec-bash.sh` logs into the plugin's container and runs bash
