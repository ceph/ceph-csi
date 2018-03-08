FROM centos:7
LABEL maintainers="Kubernetes Authors"
LABEL description="RBD CSI Plugin"

ENV CEPH_VERSION "luminous"
RUN yum  install -y centos-release-ceph && \
    yum install -y ceph-common e2fsprogs && \ 
    yum clean all

COPY _output/rbdplugin /rbdplugin
RUN chmod +x /rbdplugin
ENTRYPOINT ["/rbdplugin"]
