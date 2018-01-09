# Based on centos
FROM centos:7.4.1708
LABEL maintainers="Kubernetes Authors"
LABEL description="Cinder CSI Plugin"

# Copy cinderplugin from build directory
COPY cinderplugin /cinderplugin

# Install e4fsprogs for format
RUN yum -y install e4fsprogs

# Define default command
ENTRYPOINT ["/cinderplugin"]
