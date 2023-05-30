# Metrics

- [Metrics](#metrics)
   - [Liveness](#liveness)

## Liveness

CSI deploys a sidecar container that is responsible for collecting metrics.

Liveness metrics are intended to be collected by prometheus but can be accessed
through a GET request to a specific pod ip.

for example
`curl -X get http://[pod ip]:[liveness-port][liveness-path] 2>/dev/null | grep csi`

the expected output should be

```bash
curl -X GET http://10.109.65.142:8080/metrics 2>/dev/null | grep csi
# HELP csi_liveness Liveness Probe
# TYPE csi_liveness gauge
csi_liveness 1
```

Promethues can be deployed through the promethues operator described [here](https://coreos.com/operators/prometheus/docs/latest/user-guides/getting-started.html).
The [service-monitor](../deploy/service-monitor.yaml) will tell promethues how
to pull metrics out of CSI.

Each CSI pod has a service to expose the endpoint to prometheus. By default, rbd
pods run on port 8080 and cephfs 8081.
These can be changed if desired or if multiple ceph clusters are deployed more
ports will be used for additional CSI pods.

Note: You may need to open the ports used in your firewall depending on how your
cluster has set up.
