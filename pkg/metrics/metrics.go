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

package metrics

import (
	"context"
	"errors"
	"time"

	"k8s.io/klog"
)

var (
	serviceResponseTimes []float64
)

// Server is used to implement RequestMetrics.
type Server struct{}

// RequestMetrics handle incoming metric requests
func (s *Server) RequestMetrics(ctx context.Context, in *MetricRequest) (*MetricReply, error) {
	klog.V(3).Infof("Metric %v Requested", in.Name)

	switch in.Name {
	case "SRT":
		return returnSRT()
	default:
		return &MetricReply{Metric: nil}, errors.New("Invalid Metric Request")
	}
}

func returnSRT() (*MetricReply, error) {
	defer empty(&serviceResponseTimes)
	return &MetricReply{Metric: serviceResponseTimes}, nil
}

func empty(x *[]float64) {
	*x = nil
}

// NewMetricServer initialize a metric server
func NewMetricServer() *Server {
	return &Server{}
}

// Metric structure
type Metric struct {
	Time     time.Time
	Call     string
	SRT      float64
	Request  string
	Responce string
}

// HandleMetric collect metrics
func HandleMetric(metric Metric) {
	klog.V(3).Infof("Service responce time: %f , GRPC type: %s", metric.SRT, metric.Call)

	//dont collect srt on metric and liveness requests
	if metric.Call != "/metrics.Metrics/RequestMetrics" && metric.Call != "/csi.v1.Identity/Probe" {
		serviceResponseTimes = append(serviceResponseTimes, metric.SRT)
	}
}
