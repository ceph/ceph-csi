package main

import (
	"context"
	"log"
	"net/http"
	"time"

	metrics "github.com/ceph/ceph-csi/pkg/metrics"
	connlib "github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

const (
	address = "unix:///csi/csi.sock"
)

var (
	// create prometheus metrics
	serviceResponseTimes = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "ceph_csi",
		Name:      "srt",
		Help:      "Service Responce Times",
		Buckets:   []float64{1, 2, 5, 10, 20, 60},
	})

	liveness = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ceph_csi",
		Name:      "liveness",
		Help:      "Liveness Probe",
	})
)

func getSRT() {
	// Set up a connection to the server.
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := metrics.NewMetricsClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	r, err := c.RequestMetrics(ctx, &metrics.MetricRequest{Name: "SRT"})
	if err != nil {
		log.Fatalf("could not get SRTs: %v", err)
	}
	log.Printf("Incoming SRTs: %v", r.Metric)

	for _, element := range r.Metric {
		serviceResponseTimes.Observe(element)
	}

}

func getLiveness() {

	csiConn, err := connlib.Connect(address)
	if err != nil {
		// connlib should retry forever so a returned error should mean
		// the grpc client is misconfigured rather than an error on the network
		log.Fatalf("failed to establish connection to CSI driver: %v", err)
	}

	klog.Infof("calling CSI driver to discover driver name")
	csiDriverName, err := rpc.GetDriverName(context.Background(), csiConn)
	if err != nil {
		klog.Fatalf("failed to get CSI driver name: %v", err)
	}
	klog.Infof("CSI driver name: %q", csiDriverName)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	klog.Infof("Sending probe request to CSI driver %q", csiDriverName)
	ready, err := rpc.Probe(ctx, csiConn)
	if err != nil {
		liveness.Set(0)
		log.Printf("health check failed: %v", err)
		return
	}

	if !ready {
		liveness.Set(0)
		log.Printf("driver responded but is not ready")
		return
	}
	liveness.Set(1)
	log.Printf("Health check succeeded")
}

func recordMetrics() {
	//register promethues metrics
	prometheus.MustRegister(serviceResponseTimes)
	prometheus.MustRegister(liveness)

	go func() {
		for {
			// handle each metric
			getSRT()
			getLiveness()
			// wait to poll metrics again
			time.Sleep(15 * time.Second)
		}
	}()
}

func main() {
	// set up metrics collection
	recordMetrics()
	// set up prome server
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":8080", nil)
}
