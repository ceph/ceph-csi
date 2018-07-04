FROM ubuntu:16.04
LABEL maintainers="Kubernetes Authors"
LABEL description="CephFS CSI Plugin"

ENV CEPH_VERSION "mimic"

RUN apt-get update && apt-get install -y wget && \
    wget -q -O- 'https://download.ceph.com/keys/release.asc' | apt-key add - && \
    echo "deb http://download.ceph.com/debian-$CEPH_VERSION/ xenial main" | tee /etc/apt/sources.list.d/ceph-$CEPH_VERSION.list && \
    apt-get update && apt-get install -y kmod ceph-common ceph-fuse attr --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

COPY cephfsplugin /cephfsplugin

RUN chmod +x /cephfsplugin && \
    mkdir -p /var/log/ceph

ENTRYPOINT ["/cephfsplugin"]
