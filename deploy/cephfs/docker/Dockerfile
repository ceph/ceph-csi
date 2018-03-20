FROM ubuntu:16.04
LABEL maintainers="Kubernetes Authors"
LABEL description="CephFS CSI Plugin"

ENV CEPH_VERSION "luminous"

RUN apt-get update && \
    apt-get install -y ceph-fuse && \
    rm -rf /var/lib/apt/lists/*

COPY cephfsplugin /cephfsplugin

RUN chmod +x /cephfsplugin && \
    mkdir -p /var/log/ceph

ENTRYPOINT ["/cephfsplugin"]
