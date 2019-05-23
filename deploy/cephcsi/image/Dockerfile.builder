FROM golang:1.11 AS ceph-csi-builder
WORKDIR /go/src/github.com/ceph/ceph-csi/
COPY . .
RUN go get -u github.com/golang/dep/cmd/dep
RUN dep ensure -vendor-only
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o cephcsi ./cmd/

FROM ceph/ceph:v14.2
LABEL maintainers="Ceph-CSI Authors"
LABEL description="Ceph-CSI Plugin"

ENV CSIBIN=/usr/local/bin/cephcsi

COPY --from=ceph-csi-builder /go/src/github.com/ceph/ceph-csi/cephcsi $CSIBIN

RUN chmod +x $CSIBIN && \
    ln -sf $CSIBIN /usr/local/bin/cephcsi-rbd && \
    ln -sf $CSIBIN /usr/local/bin/cephcsi-cephfs

ENTRYPOINT ["/usr/local/bin/cephcsi"]
