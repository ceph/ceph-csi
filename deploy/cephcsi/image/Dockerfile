ARG SRC_DIR="/go/src/github.com/ceph/ceph-csi/"
ARG GO_ARCH
ARG BASE_IMAGE
ARG FINAL_BASE_IMAGE="quay.io/rockylinux/rockylinux:10-minimal"

FROM ${BASE_IMAGE} as updated_base

ARG CEPH_RELEASE_RPM

RUN rpm -ivh ${CEPH_RELEASE_RPM} \
    && dnf -y install epel-release \
    && dnf -y update --nobest \
    && dnf -y install --enablerepo=crb --setopt=install_weak_deps=False nfs-utils \
    && dnf clean all \
    && rm -rf /var/cache/dnf /var/cache/yum

FROM updated_base as builder

LABEL stage="build"

ARG CSI_IMAGE_NAME=quay.io/cephcsi/cephcsi
ARG CSI_IMAGE_VERSION=canary
ARG GO_ARCH
ARG SRC_DIR
ARG GIT_COMMIT
ARG GO_BUILD_VERBOSE
ARG GOROOT=/usr/local/go

COPY build.env /

RUN source /build.env && \
    ( test -n "${GO_ARCH}" && exit 0; echo -e "\n\nMissing GO_ARCH argument for building image, install Golang or run: make image-cephcsi GOARCH=amd64\n\n"; exit 1 ) && \
    mkdir -p ${GOROOT} && \
    curl -L https://go.dev/dl/go${GOLANG_VERSION}.linux-${GO_ARCH}.tar.gz | tar xzf - -C ${GOROOT} --strip-components=1

# test if the downloaded version of Golang works (different arch?)
RUN ${GOROOT}/bin/go version && ${GOROOT}/bin/go env

# other/conflicting versions of protobuf get installed as dependency
RUN dnf -y remove protobuf

RUN dnf -y install --enablerepo=crb --nodocs \
	librados-devel librbd-devel libcephfs-devel \
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
RUN make cephcsi GO_BUILD_VERBOSE="${GO_BUILD_VERBOSE}"

#-- Final container
FROM ${FINAL_BASE_IMAGE}

ARG SRC_DIR
ARG CEPH_RELEASE_RPM

LABEL maintainers="Ceph-CSI Authors" \
    version=${CSI_IMAGE_VERSION} \
    architecture=${GO_ARCH} \
    description="Ceph-CSI Plugin"

COPY --from=builder ${SRC_DIR}/_output/cephcsi /usr/local/bin/cephcsi

RUN true \
  && rpm -ivh ${CEPH_RELEASE_RPM} \
  && microdnf -y install kmod nfs-utils nvme-cli epel-release psmisc e2fsprogs xfsprogs cryptsetup attr --setopt=install_weak_deps=0 --setopt=tsflags=nodocs \
  && microdnf -y install --enablerepo=crb thrift librados2 librbd1 libcephfs2 ceph-common ceph-fuse rbd-nbd --setopt=install_weak_deps=0 --setopt=tsflags=nodocs \
  && microdnf clean all \
  && true

# verify that all dynamically linked libraries are available
RUN [ $(ldd /usr/local/bin/cephcsi | grep -c '=> not found') = '0' ]

ENTRYPOINT ["/usr/local/bin/cephcsi"]
