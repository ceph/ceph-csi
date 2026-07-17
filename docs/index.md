# Ceph-CSI Documentation

Welcome to the Ceph-CSI documentation. Ceph-CSI implements Container Storage
Interface (CSI) drivers for Ceph storage, enabling dynamic provisioning and
management of Ceph volumes in Kubernetes.

![Ceph Logo](assets/ceph-logo.png)

## Overview

Ceph-CSI provides CSI drivers for the following Ceph storage types:

- **RBD (RADOS Block Device)**: Block storage for Kubernetes workloads
- **CephFS**: Shared filesystem storage with ReadWriteMany support
- **NFS**: NFS-based storage provisioning
- **NVMe-oF**: NVMe over Fabrics storage

All drivers are packaged in a single binary, making deployment and management
straightforward.

## Quick Links

### Getting Started

- [Development Guide](development-guide.md) - Set up your development environment
- [Coding Conventions](coding.md) - Learn about code style and best practices

### Deployment

- [RBD Deployment Guide](rbd/deploy.md) - Deploy the RBD CSI driver
- [CephFS Deployment Guide](cephfs/deploy.md) - Deploy the CephFS CSI driver

### Features

- [Capabilities](capabilities.md) - Supported CSI features and operations
- [Snapshots and Clones](snap-clone.md) - Volume snapshot and cloning support
- [Volume Expansion](expand-pvc.md) - Expand persistent volumes dynamically
- [Static PVC](static-pvc.md) - Use pre-existing Ceph volumes
- [Metrics](metrics.md) - Monitoring and observability

### CSI-Addons

Ceph-CSI integrates with [CSI-Addons](https://csi-addons.github.io/) to provide
advanced storage operations and disaster recovery capabilities.

- [Disaster Recovery](csi-addons/disaster-recovery.md) - Volume replication
  and failover/failback procedures
- [Network Fencing](csi-addons/networkfence.md) - Network-based access
  control for storage resources
- [Reclaim Space](csi-addons/reclaimspace.md) - Space reclamation for
  filesystem volumes using fstrim
- [CSI-Addons Documentation](https://csi-addons.github.io/) - Official
  CSI-Addons documentation

### Operations

- [Upgrade Guide](ceph-csi-upgrade.md) - Upgrade Ceph-CSI to newer versions
- [In-tree Migration](intree-migrate.md) - Migrate from in-tree Ceph plugins
- [Resource Cleanup](resource-cleanup.md) - Clean up resources properly
- [Releases](releases.md) - Release process and versioning

## Architecture

Ceph-CSI implements the CSI specification with three main components:

- **IdentityServer**: Provides driver identification and capabilities
- **ControllerServer**: Manages volume lifecycle (create, delete, attach,
  detach, snapshot, clone)
- **NodeServer**: Handles node-local operations (stage, unstage, publish,
  unpublish, mount)

## Contributing

Contributions are welcome! Please see our [development
guide](development-guide.md) and [coding conventions](coding.md) to get
started.

## Support

- **GitHub Issues**: [Report bugs or request features](https://github.com/ceph/ceph-csi/issues)
- **GitHub Discussions**: [Ask questions and discuss](https://github.com/ceph/ceph-csi/discussions)
- **Ceph Community**: [Join the Ceph community](https://ceph.io/community/)

## License

Ceph-CSI is licensed under the Apache License 2.0. See the
[LICENSE](https://github.com/ceph/ceph-csi/blob/devel/LICENSE) file for details.
