# Metrics

- [Metrics](#metrics)
   - [Liveness](#liveness)
   - [Profiling](#profiling)

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

Prometheus can be deployed through the prometheus operator described [here](https://coreos.com/operators/prometheus/docs/latest/user-guides/getting-started.html).
The [service-monitor](../deploy/service-monitor.yaml) will tell prometheus how
to pull metrics out of CSI.

Each CSI pod has a service to expose the endpoint to prometheus. By default, rbd
pods run on port 8080 and cephfs 8081.
These can be changed if desired or if multiple ceph clusters are deployed more
ports will be used for additional CSI pods.

Note: You may need to open the ports used in your firewall depending on how your
cluster has set up.

## Profiling

Ceph-CSI has built-in support for
[Golang profiling](https://go.dev/blog/pprof). Profiling is mainly useful for
debugging problems like high CPU usage, memory leaks and goroutine leaks.

Profiling is disabled by default and can be enabled with the
`--enableprofiling` command line option. The deployment manifests in `deploy/`
pass `--enableprofiling=false` for the containers that set the option, and it
needs to be changed to `true` for the container that needs debugging, for
example in `deploy/rbd/kubernetes/csi-rbdplugin-provisioner.yaml`:

```yaml
args:
  - "--nodeid=$(NODE_ID)"
  - "--type=rbd"
  - "--controllerserver=true"
  - "--enableprofiling=true"
```

The profiling endpoints are served by the same HTTP server that serves the
`/metrics` endpoint, so they are available on the `--metricsport` port (8080
by default) of the Ceph-CSI container.

Note: profiles contain details about the internals of the process, like
function names, memory addresses and command line arguments. Enabling
profiling in production should be done with care, and preferably only while
debugging a particular problem.

### Fetching profiles

The profiles are available under
[`/debug/pprof/`](https://pkg.go.dev/net/http/pprof), a list of all endpoints
is returned when requesting `/debug/pprof/` itself. The commonly used
endpoints are:

| Endpoint                       | Profile                                                        |
| ------------------------------ | -------------------------------------------------------------- |
| `/debug/pprof/profile`         | CPU profile, accepts `?seconds=N` for the duration (default 30) |
| `/debug/pprof/heap`            | Memory allocations of objects that are still in use, accepts `?gc=1` to run GC first |
| `/debug/pprof/allocs`          | Sample of all memory allocations made since the process started |
| `/debug/pprof/goroutine`       | Stack traces of all goroutines                                 |
| `/debug/pprof/block`           | Stack traces that led to blocking on channels or mutexes, collection is not enabled by `--enableprofiling` |
| `/debug/pprof/mutex`           | Stack traces of holders of contended mutexes, collection is not enabled by `--enableprofiling` |
| `/debug/pprof/trace`           | Execution trace, accepts `?seconds=N` for the duration           |

Profiles of a Ceph-CSI Pod that runs in Kubernetes can be fetched with
`kubectl port-forward`, `go tool pprof` is used to analyse them:

```bash
# find the Pod that needs debugging
kubectl -n ceph-csi get pods

# forward the metrics port of the Ceph-CSI container
kubectl -n ceph-csi port-forward csi-rbdplugin-provisioner-xxxxxxxxx-xxxxx 8080:8080

# in another terminal: fetch a CPU profile and inspect it
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
```

`go tool pprof` accepts an URL, downloads the profile and opens an
interactive session. The `top` command lists the functions that consume the
most CPU time, `web` renders the profile as a call-graph (requires
[graphviz](https://graphviz.org/)). Passing `-http=localhost:8081` opens the
profile in a web browser instead, where a flame graph view is available.

The profiles contain the function names and line numbers of the running
Ceph-CSI revision, so they can be analysed without additional files. Passing
the `cephcsi` binary that was built from the same sources adds more details,
like the disassembly of the functions in the profile:

```bash
# build the binary from the same source revision, in a container that
# provides the required Ceph libraries and build tags
make containerized-build

go tool pprof _output/cephcsi heap2.pprof
```

An execution trace can be inspected in a web browser with `go tool trace`:

```bash
curl -o trace.out "http://localhost:8080/debug/pprof/trace?seconds=10"
go tool trace trace.out
```

### Debugging memory leaks

For suspected memory leaks, two heap profiles taken at different moments can
be compared. The `-base` option subtracts the sample values of the first
profile from the second, grouped by allocation stack, which shows the net
change in live memory between the two moments:

```bash
curl -o heap1.pprof http://localhost:8080/debug/pprof/heap
# ... let the process run while the memory usage grows ...
curl -o heap2.pprof http://localhost:8080/debug/pprof/heap

# analyse the difference between the two profiles
go tool pprof -base heap1.pprof heap2.pprof
```

Goroutine leaks can be found by dumping the stack traces of all goroutines
with `/debug/pprof/goroutine?debug=1`, goroutines stuck in the same call
stack are grouped and counted:

```bash
curl http://localhost:8080/debug/pprof/goroutine?debug=1
```
