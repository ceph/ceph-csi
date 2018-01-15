FROM centos:7.4.1708

# Copy nfsplugin from build _output directory
COPY nfsplugin /nfsplugin

RUN yum -y install nfs-utils && yum -y install epel-release && yum -y install jq && yum clean all

ENTRYPOINT ["/nfsplugin"]
