# DEPRECATION NOTICE

**These manual YAML deployment manifests are DEPRECATED and will be removed in Ceph-CSI v3.19.**

## Recommended Migration Path

The [Ceph-CSI Operator](https://ceph.github.io/ceph-csi-operator) is now the officially supported deployment method for Kubernetes. The operator provides:

- **Automated lifecycle management**: Install, upgrade, and configure Ceph-CSI drivers automatically
- **Simplified operations**: Declarative configuration through Custom Resources
- **Better upgrade experience**: Seamless driver upgrades with minimal downtime
- **Consistent deployment**: Ensures best practices and correct configuration

## Migration Instructions

Please refer to the [Ceph-CSI Operator documentation](https://ceph.github.io/ceph-csi-operator) for installation and migration guidance.

## Support Timeline

- **v3.18 (current)**: Manual YAML deployments are deprecated but still present in the repository (untested)
- **v3.19**: Manual YAML deployment files will be removed

If you have questions or need assistance migrating to the operator, please reach out on:
- Slack: [#ceph-csi channel](https://ceph-storage.slack.com/archives/C05522L7P60)
- GitHub: [Ceph-CSI Issues](https://github.com/ceph/ceph-csi/issues)
