FROM ceph/ceph:v14.2.2-20190826
LABEL maintainers="Ceph-CSI Authors"
LABEL description="Ceph-CSI Plugin"

ENV CSIBIN=/usr/local/bin/cephcsi

RUN yum -y update && yum clean all
COPY cephcsi $CSIBIN

RUN chmod +x $CSIBIN

ENTRYPOINT ["/usr/local/bin/cephcsi"]
