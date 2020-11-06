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
	"net"
	"net/http"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	connlib "github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type probeConn struct {
	conn   *grpc.ClientConn
	config *util.Config
}

var (
	liveness = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "csi",
		Name:      "liveness",
		Help:      "Liveness Probe",
	})
)

func (c *probeConn) checkProbe(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), c.config.ProbeTimeout)
	defer cancel()

	util.TraceLog(ctx, "Healthz req: Sending probe request to CSI driver %s", c.config.DriverName)
	ready, err := rpc.Probe(ctx, c.conn)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write([]byte(err.Error()))
		if err != nil {
			util.ErrorLog(ctx, "Healthz req: write failed: %v", err)
		}
		util.ErrorLog(ctx, "Healthz req: health check failed: %v", err)
		return
	}

	if !ready {
		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write([]byte("Healthz req: driver responded but is not ready"))
		if err != nil {
			util.ErrorLog(ctx, "Healthz req: write failed: %v", err)
		}

		util.ErrorLog(ctx, "Healthz req: driver responded but is not ready")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(`ok`))
	if err != nil {
		util.ErrorLog(ctx, "Healthz req: write failed: %v", err)
	}
	util.ExtendedLog(ctx, "Healthz req: Health check succeeded")
}

func getLiveness(c *probeConn) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.ProbeTimeout)
	defer cancel()

	util.TraceLog(ctx, "Metrics req: Sending probe request to CSI driver: %s", c.config.DriverName)
	ready, err := rpc.Probe(ctx, c.conn)
	if err != nil {
		liveness.Set(0)
		util.ErrorLog(ctx, "Metrics req: health check failed: %v", err)
		return
	}

	if !ready {
		liveness.Set(0)
		util.ErrorLog(ctx, "Metrics req: driver responded but is not ready")
		return
	}
	liveness.Set(1)
	util.ExtendedLog(ctx, "Metrics req: Health check succeeded")
}

func recordLiveness(c *probeConn) {
	// register prometheus metrics
	err := prometheus.Register(liveness)
	if err != nil {
		util.FatalLogMsg(err.Error())
	}

	// get liveness periodically
	ticker := time.NewTicker(c.config.PollTime)
	defer ticker.Stop()
	for range ticker.C {
		getLiveness(c)
	}
}

// Run starts liveness collection and prometheus endpoint.
func Run(conf *util.Config) {
	util.ExtendedLogMsg("Liveness Running")

	liveMetricsManager := metrics.NewCSIMetricsManager("")

	csiConn, err := connlib.Connect(conf.Endpoint, liveMetricsManager)
	if err != nil {
		// connlib should retry forever so a returned error should mean
		// the grpc client is misconfigured rather than an error on the network
		util.FatalLogMsg("failed to establish connection to CSI driver: %v", err)
	}

	conf.DriverName, err = rpc.GetDriverName(context.Background(), csiConn)
	if err != nil {
		util.FatalLogMsg("failed to get CSI driver name: %v", err)
	}
	liveMetricsManager.SetDriverName(conf.DriverName)
	util.ExtendedLogMsg("CSI driver: %s, Endpoint: %s", conf.DriverName, conf.Endpoint)

	pc := &probeConn{
		config: conf,
		conn:   csiConn,
	}

	// start liveness collection
	go recordLiveness(pc)

	// start up prometheus endpoint
	util.StartMetricsServer(conf)

	address := net.JoinHostPort(conf.MetricsIP, conf.HealthzPort)
	http.HandleFunc(conf.HealthzPath, pc.checkProbe)
	util.ExtendedLogMsg("Serving Health requests on: http://%s%s", address, conf.HealthzPath)
	err = http.ListenAndServe(address, nil)
	if err != nil {
		util.FatalLogMsg("failed to listen on address %s: %v", address, err)
	}
}
