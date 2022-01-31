ARG SRC_DIR="/go/src/github.com/ceph/ceph-csi/"
ARG GO_ARCH
ARG BASE_IMAGE

FROM ${BASE_IMAGE} as builder

# FIXME: Ceph base containers should use CentOS Stream 8
# Move the container to CentOS Stream 8. Do this as 1st step in the
# container-image creation, so that the final container-image can use the
# cached layer.
RUN true \
    && dnf -y swap --disablerepo='*' --repofrompath=c8-extras,http://mirror.centos.org/centos/8/extras/x86_64/os centos-linux-repos centos-stream-repos \
    && dnf -y distro-sync \
    && true

LABEL stage="build"

ARG CSI_IMAGE_NAME=quay.io/cephcsi/cephcsi
ARG CSI_IMAGE_VERSION=canary
ARG GO_ARCH
ARG SRC_DIR
ARG GIT_COMMIT
ARG GOROOT=/usr/local/go

COPY build.env /

RUN source /build.env && \
    ( test -n "${GO_ARCH}" && exit 0; echo -e "\n\nMissing GO_ARCH argument for building image, install Golang or run: make image-cephcsi GOARCH=amd64\n\n"; exit 1 ) && \
    mkdir -p ${GOROOT} && \
    curl https://storage.googleapis.com/golang/go${GOLANG_VERSION}.linux-${GO_ARCH}.tar.gz | tar xzf - -C ${GOROOT} --strip-components=1

# test if the downloaded version of Golang works (different arch?)
RUN ${GOROOT}/bin/go version && ${GOROOT}/bin/go env

RUN dnf -y install \
	librados-devel librbd-devel \
	/usr/bin/cc \
	make \
	git \
    && true

ENV GOROOT=${GOROOT} \
    GOPATH=/go \
    CGO_ENABLED=1 \
    GIT_COMMIT="${GIT_COMMIT}" \
    ENV_CSI_IMAGE_VERSION="${CSI_IMAGE_VERSION}" \
    ENV_CSI_IMAGE_NAME="${CSI_IMAGE_NAME}" \
    PATH="${GOROOT}/bin:${GOPATH}/bin:${PATH}"


WORKDIR ${SRC_DIR}

# Copy source directories
COPY . ${SRC_DIR}

# Build executable
RUN make cephcsi

#-- Final container
FROM ${BASE_IMAGE}

# FIXME: Ceph base containers should use CentOS Stream 8
# Move the container to CentOS Stream 8
RUN true \
    && dnf -y swap --disablerepo='*' --repofrompath=c8-extras,http://mirror.centos.org/centos/8/extras/x86_64/os centos-linux-repos centos-stream-repos \
    && dnf -y distro-sync \
    && true

ARG SRC_DIR

LABEL maintainers="Ceph-CSI Authors" \
    version=${CSI_IMAGE_VERSION} \
    architecture=${GO_ARCH} \
    description="Ceph-CSI Plugin"

COPY --from=builder ${SRC_DIR}/_output/cephcsi /usr/local/bin/cephcsi

# verify that all dynamically linked libraries are available
RUN [ $(ldd /usr/local/bin/cephcsi | grep -c '=> not found') = '0' ]

ENTRYPOINT ["/usr/local/bin/cephcsi"]
