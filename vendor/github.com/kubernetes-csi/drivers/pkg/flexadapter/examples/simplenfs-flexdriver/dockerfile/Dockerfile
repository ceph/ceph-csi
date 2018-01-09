FROM centos:7.4.1708

# Copy flexadapter from build _output directory
COPY flexadapter /flexadapter
# Copy nfs from driver directory
COPY nfs /drivers/nfs

RUN yum -y install nfs-utils && yum -y install epel-release && yum -y install jq && yum clean all

ENTRYPOINT ["/flexadapter", "--driverpath=/drivers/nfs"]
