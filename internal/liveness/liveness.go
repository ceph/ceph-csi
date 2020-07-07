/*
Copyright 2019 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package liveness

import (
	"context"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	connlib "github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

var (
	liveness = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "csi",
		Name:      "liveness",
		Help:      "Liveness Probe",
	})
)

func getLiveness(timeout time.Duration, csiConn *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	klog.V(5).Info("Sending probe request to CSI driver") // nolint:gomnd // number specifies log level
	ready, err := rpc.Probe(ctx, csiConn)
	if err != nil {
		liveness.Set(0)
		klog.Errorf("health check failed: %v", err)
		return
	}

	if !ready {
		liveness.Set(0)
		klog.Error("driver responded but is not ready")
		return
	}
	liveness.Set(1)
	klog.V(3).Infof("Health check succeeded") // nolint:gomnd // number specifies log level
}

func recordLiveness(endpoint, drivername string, pollTime, timeout time.Duration) {
	liveMetricsManager := metrics.NewCSIMetricsManager(drivername)
	// register prometheus metrics
	err := prometheus.Register(liveness)
	if err != nil {
		klog.Fatalln(err)
	}

	csiConn, err := connlib.Connect(endpoint, liveMetricsManager)
	if err != nil {
		// connlib should retry forever so a returned error should mean
		// the grpc client is misconfigured rather than an error on the network
		klog.Fatalf("failed to establish connection to CSI driver: %v", err)
	}

	// get liveness periodically
	ticker := time.NewTicker(pollTime)
	defer ticker.Stop()
	for range ticker.C {
		getLiveness(timeout, csiConn)
	}
}

// Run starts liveness collection and prometheus endpoint.
func Run(conf *util.Config) {
	klog.V(3).Infof("Liveness Running") // nolint:gomnd // number specifies log level

	// start liveness collection
	go recordLiveness(conf.Endpoint, conf.DriverName, conf.PollTime, conf.PoolTimeout)

	// start up prometheus endpoint
	util.StartMetricsServer(conf)
}
