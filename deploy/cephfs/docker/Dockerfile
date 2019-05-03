FROM ceph/ceph:v14.2
LABEL maintainers="Ceph-CSI Authors"
LABEL description="Ceph-CSI Plugin"

ENV CSIBIN=/usr/local/bin/cephcsi

COPY cephfsplugin $CSIBIN

RUN chmod +x $CSIBIN && \
    ln -sf $CSIBIN /usr/local/bin/cephcsi-cephfs

ENTRYPOINT ["/usr/local/bin/cephcsi-cephfs"]
