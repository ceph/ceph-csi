# Configure Snapshot-based RBD Mirroring

---
RBD mirroring is an asynchronous replication of RBD images between multiple
Ceph clusters. The following guide demonstrates how to perform the basic
administrative tasks to configure
[snapshot-based mirroring](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/#create-image-mirror-snapshots)
using the rbd command and handle Disaster Recovery scenarios.

For more details about RBD Mirroring, refer to the official
[Ceph documentation](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/).

## Requirements

* [Rook](https://rook.io/docs/rook/master/) should be installed
  on the kubernetes cluster.
  Refer [this](https://rook.io/docs/rook/master/ceph-quickstart.html) for rook installation.

* Two Ceph clusters, such that, nodes on both clusters can connect to the nodes
  in the other cluster.

  > Note: snapshot-based mirroring requires the Ceph [Octopus](https://docs.ceph.com/en/latest/releases/)
release or later.

## Create Ceph Pools

* Create a Ceph pool. For that, create `pool-test.yaml`

   ```yaml
    apiVersion: ceph.rook.io/v1
    kind: CephBlockPool
    metadata:
      name: replicapool
      namespace: rook-ceph
    spec:
      replicated:
        size: 1
      mirroring:
        enabled: true
        mode: image
      statusCheck:
        mirror:
          disabled: false
          interval: 60s
    ```

* Creating `replicapool` pool on primary cluster:

  `[cluster-a]$ kubectl create -f pool-test.yaml --context=cluster-a`

* Also, similar pool on the secondary cluster:

  `[cluster-b]$ kubectl create -f pool-test.yaml --context=cluster-b`

> **Note**: Pools in both the clusters should have the same name.

## Bootstrap Peers

* For Bootstrapping peer, the secret name is required. This is the secret
  which will be provided to it's peer.

    ```sh
    [cluster-a]$ kubectl get cephblockpool.ceph.rook.io/replicapool -nrook-ceph --context=cluster-a -oyaml
    ```

    Here, the secret information is present in `rbdMirrorBootstrapPeerSecretName`
attribute:

   ```sh
    status:
        info:
        rbdMirrorBootstrapPeerSecretName: pool-peer-token-replicapool
    ```

* Fetch the Kubernetes secret information

    ```sh
    [cluster-b]$ kuberc get secrets pool-peer-token-replicapool --context=cluster-b -oyaml
    ```

* The secret `pool-peer-token-replicapool` contains all the information
  related to the token and needs to be fed to the peer, but it is encoded.
  To fetch the decoded secret:

    ```sh
    [cluster-b]$ kuberc get secrets pool-peer-token-replicapool --context=cluster-b -o jsonpath='{.data.token}'|base64 -d
    ```

* With this Decoded value, create a secret on the primary site(cluster-a),
  using site name of the peer as the name.

    ```sh
    [cluster-a]$ kubectl -n rook-ceph create secret generic f4df4694-342f-4e77-9d1e-c97014a95818-rook-ceph --from-literal=token=eyJmc2lkIjoiZjRkZjQ2OTQtMzQyZi00ZTc3LTlkMWUtYzk3MDE0YTk1ODE4IiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFDSThYNWZoRUtaTlJBQXdCYTdwekY2aTN4V0oyL2JPSmlBN3c9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMzkuNDk6MzMwMCx2MToxOTIuMTY4LjM5LjQ5OjY3ODldIn0= --from-literal=pool=replicapool --context=cluster-a
    ```

    > Note: The command might be too long to directly run on cli,
    > an alternate approach can be to save token in a file

Now the kubernetes **secret is created on the primary site** (cluster-a),
which contains token information required to connect with its peer i.e cluster-b.
Now we have successfully registered cluster-b to cluster-a as a peer.
Since it is two-way replication, **follow similar steps to on the peer to
perform vice-versa**.

## Create RBDMirror CRD

Replication is handled by **rbd-mirror** daemon. The rbd-mirror daemon is responsible
for pulling image updates from the remote, peer cluster, and applying them to image
within the local cluster. Rook allows the creation and updating rbd-mirror daemon(s)
through the custom resource definitions (CRDs).

* Create `mirror.yaml`, to deploy the rbd-mirror daemon

    ```yaml
    apiVersion: ceph.rook.io/v1
    kind: CephRBDMirror
    metadata:
      name: my-rbd-mirror
      namespace: rook-ceph
    spec:
      # the number of rbd-mirror daemons to deploy
      count: 1
      peers:
        secretNames:
          # list of Kubernetes Secrets containing the peer token
          - "f4df4694-342f-4e77-9d1e-c97014a95818-rook-ceph"
    ```

* Create the RBD mirror daemon

  `[cluster-A]$ kubectl create -f mirror.yaml --context=cluster-a`

* Validate if rbd-mirror daemon pod is now up

  `[cluster-A]$ kuberc get pods --context=cluster-a`

* Verify that daemon health is OK

    ```sh
    [cluster-A]$ kubectl get cephblockpool.ceph.rook.io/replicapool -nrook-ceph --context=cluster-a -oyaml
    ```

**Follow similar steps to on the peer to perform vice-versa.**

## Enable Mirroring

Once the rbd-mirror daemon is running on both the clusters, follow the below steps:

* First, `ssh` into the toolbox pods of both clusters to check the pool status

  `[cluster-a toolbox]$ rbd mirror pool status --pool=replicapool`
  `[cluster-b toolbox]$ rbd mirror pool status --pool=replicapool`

* Create an rbd image in cluster-a

    ```sh
    [cluster-a toolbox]$ rbd create pvcname-namespace-xxx-xxx-xxx --size=1024 --pool=replicapool
    ```

* Verify if the image is created:

    ```sh
    [cluster-a toolbox]$ rbd image ls --pool=replicapool or rbd ls --pool=replicapool
    ```

* Enable mirroring on the created image

    ```sh
    [cluster-a toolbox] rbd mirror image enable pvcname-namespace-xxx-xxx-xxx snapshot --pool=replicapool
    ```

    > Note: Mirrored RBD images are designated as either primary or non-primary.
Images are automatically promoted to primary when mirroring is
first enabled on an image.

* Check whether the mirrored image is now present in the secondary cluster.

    `[cluster-b toolbox]$ rbd ls --pool=replicapool`

    The mirrored image should be visible on the secondary cluster now.

* To fetch additional info about image:

    `[cluster-b toolbox]# rbd info pvcname-namespace-xxx-xxx-xxx --pool=replicapool`

  ```sh
  O/P:
  rbd image 'pvcname-namespace-xxx-xxx-xxx':
  size 1 GiB in 256 objects
  order 22 (4 MiB objects)
  snapshot_count: 1
  id: 190d48946362
  block_name_prefix: rbd_data.190d48946362
  format: 2
  features: layering, non-primary
  op_features:
  flags:
    create_timestamp: Thu Oct  8 14:19:35 2020
    access_timestamp: Thu Oct  8 14:19:35 2020
    modify_timestamp: Thu Oct  8 14:19:35 2020
    mirroring state: enabled
    mirroring mode: snapshot
    mirroring global id: 14e42673-a2ad-438a-aa5e-5b4fb59f1d8f
    mirroring primary: false
    ```

Since the image on `cluster-b` is "non-primary"; Notice that, here:

1. `mirroring primary` attribute is `false`
1. Also, take a look at its features; one of the features is `non-primary`.
   This depicts that the image here is not primary and cannot be directly altered.

> **Note**: Since mirroring is configured in image mode for the image’s pool,
> it is necessary to explicitly enable mirroring for each image within the pool

---

## Managing Disaster Recovery

To shift the workload from primary to the secondary site,
there are two possible use cases:

* **Planned Migration**: The primary cluster is shutdown properly.

* **Disaster recovery**: The primary cluster went down without proper shutdown.

The guide assumes we have a setup of two sites, namely:

* `cluster-a`(primary site) and

* `cluster-b`(secondary site)

The below mentioned steps demonstrate how we can handle both the use cases
using snapshot-based rbd mirroring:

## Planned Migration

### Failover (transition from primary to secondary site)

In the case of Failover, access to the image on the primary site should be stopped.
The image should now be made primary on the secondary cluster, so that the access
can be resumed there.

* Demote the image on the primary site

  ```sh
  [cluster-a toolbox]$ rbd mirror image demote poolname/imagename
  ```

* Shutdown the primary site.
* Promote the images on the secondary site

  ```sh
  [cluster-b toolbox]$ rbd mirror image promote poolname/imagename
  ```

* Perform I/O on the image from the secondary cluster now.

### Failback (transition back to primary from the secondary site)

Validate if cluster-a is back online. If yes, proceed with the below-mentioned steps:

* Demote the image on the cluster-b

  ```sh
  [cluster-b toolbox]$ rbd mirror image demote poolname/imagename
  ```

* Promote the images on the cluster-a

  ```sh
  [cluster-a toolbox]$ rbd mirror image promote poolname/imagename
  ```

* The image on cluster-a is now ready to be performed I/O on.

## Disaster Recovery

### Failover

The guide assumes that the primary site (`cluster-a`) is down, and the workload now
needs to be transferred to the secondary site (`cluster-b`)

* Force promote the image on the cluster-b

  ```sh
  [cluster-b toolbox]$ rbd mirror image promote poolname/imagename --force
  ```

* The image on cluster-b is now ready to be performed I/O on.

### Failback

In case of failback, the images of primary cluster cannot be directly used,
as the images on primary and secondary clusters might not be in sync.
Thus, we need to demote the images and re-sync images on the primary cluster
before using them. Validate if cluster-a is back online. If yes, proceed
with the below-mentioned steps:

* Demote the images

    ```sh
    [cluster-a toolbox]$ rbd mirror image demote poolname/imagename
    ```

* Re-sync the images

    ```sh
    [cluster-a toolbox]$ rbd mirror image resync poolname/imagename
    ```

* Demote the images on cluster-b as now we want to shift back the
  workload to cluster-a

    ```sh
    [cluster-b toolbox]$ rbd mirror image demote poolname/imagename
    ```

* Promote the images on cluster-a to resume I/0

    ```sh
    [cluster-a toolbox]$ rbd mirror image promote poolname/imagename
    ```

