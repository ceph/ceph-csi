FROM ceph/ceph:v14.2
LABEL maintainers="Ceph-CSI Authors"
LABEL description="Ceph-CSI Plugin"

ENV CSIBIN=/usr/local/bin/cephcsi

COPY cephcsi $CSIBIN

RUN chmod +x $CSIBIN

ENTRYPOINT ["/usr/local/bin/cephcsi"]
