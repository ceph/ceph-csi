FROM quay.io/centos/centos:stream9

# FIXME #3946: jenkins-job-builder does not seem to work with urllib3 2.x
RUN true \
 && dnf -y install git make python3-pip \
 && pip3 install jenkins-job-builder 'urllib3>=1.26.15,<2' \
 && dnf -y clean all \
 && true

ENV MAKE_TARGET=${MAKE_TARGET:-test}

# Environment that needs to be set before executing checkout-repo.sh
# ENV GIT_REPO=https://github.com/ceph/ceph-csi
# ENV GIT_REF=ci/centos
ADD checkout-repo.sh /opt/build/

# make WORKDIR writable, otherwise git checkout fails
RUN chmod ugo=rwx /opt/build

ENV HOME=/opt/build
WORKDIR /opt/build

CMD ["sh", "-c", "./checkout-repo.sh && make -C deploy ${MAKE_TARGET}"]
