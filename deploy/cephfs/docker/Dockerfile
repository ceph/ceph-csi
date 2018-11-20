FROM centos:7
LABEL maintainers="Kubernetes Authors"
LABEL description="CephFS CSI Plugin"

ENV CEPH_VERSION "mimic"
RUN yum  install -y centos-release-ceph && \
    yum install -y kmod ceph-common ceph-fuse attr && \
    yum clean all

COPY cephfsplugin /cephfsplugin

RUN chmod +x /cephfsplugin && \
    mkdir -p /var/log/ceph

ENTRYPOINT ["/cephfsplugin"]
