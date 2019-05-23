FROM centos:7
LABEL maintainers="Kubernetes Authors"
LABEL description="RBD CSI Plugin"

ENV CEPH_VERSION "mimic"
RUN yum  install -y centos-release-ceph && \
    yum install -y ceph-common e2fsprogs xfsprogs rbd-nbd && \
    yum clean all

COPY --from=ceph-csi-builder /go/src/github.com/ceph/ceph-csi/cephcsi /rbdplugin
RUN chmod +x /rbdplugin
ENTRYPOINT ["/rbdplugin"]
