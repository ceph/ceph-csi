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

package csicommon

import (
	"time"

	"k8s.io/klog"

	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	serviceResponseTimes = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "ceph_csi",
		Name:      "srt",
		Help:      "Service Responce Times",
		Buckets:   []float64{1, 2, 5, 10, 20, 60},
	})
)

// Metric structure
type Metric struct {
	Time     time.Time
	Call     string
	SRT      float64
	Request  string
	Responce string
}

// InitMetrics func
func InitMetrics() {
	klog.V(3).Infof("Init Metrics Collection")

	http.Handle("/metrics", promhttp.Handler())

	go func() {
		if err := http.ListenAndServe(":8080", nil); err != http.ErrServerClosed {
			klog.V(3).Infof("ListenAndServe(): %s", err)
		}
	}()

	prometheus.MustRegister(serviceResponseTimes)
}

func handleMetric(metric Metric) {
	klog.V(3).Infof("Service responce time: %f , GRPC type: %s", metric.SRT, metric.Call)

	serviceResponseTimes.Observe(metric.SRT)
}
