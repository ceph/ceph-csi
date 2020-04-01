FROM centos:latest

RUN true \
 && yum -y install git make python3-pip \
 && pip3 install jenkins-job-builder \
 && yum -y clean all \
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
