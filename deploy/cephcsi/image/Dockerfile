ARG SRC_DIR="/go/src/github.com/ceph/ceph-csi/"
ARG GO_ARCH
ARG BASE_IMAGE

FROM ${BASE_IMAGE} as updated_base

RUN dnf -y update \
       && dnf clean all \
       && rm -rf /var/cache/yum

FROM updated_base as builder

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

# TODO: remove the following cmd, when issue
# https://github.com/ceph/ceph-container/issues/2034 is fixed.
RUN dnf config-manager --disable \
    tcmu-runner,tcmu-runner-source,tcmu-runner-noarch || true

RUN dnf -y install --nodocs \
	librados-devel librbd-devel \
	/usr/bin/cc \
	make \
	git \
	&& dnf clean all \
	&& rm -rf /var/cache/yum \
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
FROM updated_base

ARG SRC_DIR

LABEL maintainers="Ceph-CSI Authors" \
    version=${CSI_IMAGE_VERSION} \
    architecture=${GO_ARCH} \
    description="Ceph-CSI Plugin"

COPY --from=builder ${SRC_DIR}/_output/cephcsi /usr/local/bin/cephcsi

# verify that all dynamically linked libraries are available
RUN [ $(ldd /usr/local/bin/cephcsi | grep -c '=> not found') = '0' ]

ENTRYPOINT ["/usr/local/bin/cephcsi"]
