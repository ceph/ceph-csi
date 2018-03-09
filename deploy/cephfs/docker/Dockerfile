FROM ubuntu:16.04
LABEL maintainers="Kubernetes Authors"
LABEL description="CephFS CSI Plugin"

ENV CEPH_VERSION "luminous"

RUN apt-get update && \
    apt-get install -y ceph-common ceph-fuse && \
    rm -rf /var/lib/apt/lists/*

COPY cephfsplugin /cephfsplugin
COPY cephfs_provisioner.py /cephfs_provisioner.py
RUN chmod +x /cephfsplugin && chmod +x /cephfs_provisioner.py
ENTRYPOINT ["/cephfsplugin"]
