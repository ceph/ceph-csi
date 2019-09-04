FROM ceph/ceph:v14.2
LABEL maintainers="Ceph-CSI Authors"
LABEL description="Ceph-CSI Plugin"

# Removing ceph-iscsi repo to workaround the repo issue while upgrading
#RUN rm -f /etc/yum.repos.d/ceph-iscsi.repo && yum -y update && yum clean all
ENV CSIBIN=/usr/local/bin/cephcsi

COPY cephcsi $CSIBIN

RUN chmod +x $CSIBIN

ENTRYPOINT ["/usr/local/bin/cephcsi"]
