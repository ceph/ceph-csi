/*
Copyright 2019 The Kubernetes Authors.

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
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/component-base/metrics"
	"k8s.io/klog"
)

const (
	// Common metric strings
	subsystem             = "csi_sidecar"
	labelCSIDriverName    = "driver_name"
	labelCSIOperationName = "method_name"
	labelGrpcStatusCode   = "grpc_status_code"
	unknownCSIDriverName  = "unknown-driver"

	// CSI Operation Latency with status code total - Histogram Metric
	operationsLatencyMetricName = "operations_seconds"
	operationsLatencyHelp       = "Container Storage Interface operation duration with gRPC error code status total"
)

var (
	operationsLatencyBuckets = []float64{.1, .25, .5, 1, 2.5, 5, 10, 15, 25, 50, 120, 300, 600}
)

// CSIMetricsManager exposes functions for recording metrics for CSI operations.
type CSIMetricsManager interface {
	// GetRegistry() returns the metrics.KubeRegistry used by this metrics manager.
	GetRegistry() metrics.KubeRegistry

	// RecordMetrics must be called upon CSI Operation completion to record
	// the operation's metric.
	// operationName - Name of the CSI operation.
	// operationErr - Error, if any, that resulted from execution of operation.
	// operationDuration - time it took for the operation to complete
	RecordMetrics(
		operationName string,
		operationErr error,
		operationDuration time.Duration)

	// SetDriverName is called to update the CSI driver name. This should be done
	// as soon as possible, otherwise metrics recorded by this manager will be
	// recorded with an "unknown-driver" driver_name.
	// driverName - Name of the CSI driver against which this operation was executed.
	SetDriverName(driverName string)

	// StartMetricsEndpoint starts the metrics endpoint at the specified address/path
	// for this metrics manager.
	// If the metricsAddress is an empty string, this will be a no op.
	StartMetricsEndpoint(metricsAddress, metricsPath string)
}

// NewCSIMetricsManager creates and registers metrics for for CSI Sidecars and
// returns an object that can be used to trigger the metrics.
// driverName - Name of the CSI driver against which this operation was executed.
//              If unknown, leave empty, and use SetDriverName method to update later.
func NewCSIMetricsManager(driverName string) CSIMetricsManager {
	cmm := csiMetricsManager{
		registry: metrics.NewKubeRegistry(),
		csiOperationsLatencyMetric: metrics.NewHistogramVec(
			&metrics.HistogramOpts{
				Subsystem:      subsystem,
				Name:           operationsLatencyMetricName,
				Help:           operationsLatencyHelp,
				Buckets:        operationsLatencyBuckets,
				StabilityLevel: metrics.ALPHA,
			},
			[]string{labelCSIDriverName, labelCSIOperationName, labelGrpcStatusCode},
		),
	}

	cmm.SetDriverName(driverName)
	cmm.registerMetrics()
	return &cmm
}

var _ CSIMetricsManager = &csiMetricsManager{}

type csiMetricsManager struct {
	registry                   metrics.KubeRegistry
	driverName                 string
	csiOperationsMetric        *metrics.CounterVec
	csiOperationsLatencyMetric *metrics.HistogramVec
}

func (cmm *csiMetricsManager) GetRegistry() metrics.KubeRegistry {
	return cmm.registry
}

// RecordMetrics must be called upon CSI Operation completion to record
// the operation's metric.
// operationName - Name of the CSI operation.
// operationErr - Error, if any, that resulted from execution of operation.
// operationDuration - time it took for the operation to complete
func (cmm *csiMetricsManager) RecordMetrics(
	operationName string,
	operationErr error,
	operationDuration time.Duration) {
	cmm.csiOperationsLatencyMetric.WithLabelValues(
		cmm.driverName, operationName, getErrorCode(operationErr)).Observe(operationDuration.Seconds())
}

// SetDriverName is called to update the CSI driver name. This should be done
// as soon as possible, otherwise metrics recorded by this manager will be
// recorded with an "unknown-driver" driver_name.
func (cmm *csiMetricsManager) SetDriverName(driverName string) {
	if driverName == "" {
		cmm.driverName = unknownCSIDriverName
	} else {
		cmm.driverName = driverName
	}
}

// StartMetricsEndpoint starts the metrics endpoint at the specified address/path
// for this metrics manager  on a new go routine.
// If the metricsAddress is an empty string, this will be a no op.
func (cmm *csiMetricsManager) StartMetricsEndpoint(metricsAddress, metricsPath string) {
	if metricsAddress == "" {
		klog.Warningf("metrics endpoint will not be started because `metrics-address` was not specified.")
		return
	}

	http.Handle(metricsPath, metrics.HandlerFor(
		cmm.GetRegistry(),
		metrics.HandlerOpts{
			ErrorHandling: metrics.ContinueOnError}))

	// Spawn a new go routine to listen on specified endpoint
	go func() {
		err := http.ListenAndServe(metricsAddress, nil)
		if err != nil {
			klog.Fatalf("Failed to start prometheus metrics endpoint on specified address (%q) and path (%q): %s", metricsAddress, metricsPath, err)
		}
	}()
}

// VerifyMetricsMatch is a helper function that verifies that the expected and
// actual metrics are identical excluding metricToIgnore.
// This method is only used by tests. Ideally it should be in the _test file,
// but *_test.go files are compiled into the package only when running go test
// for that package and this method is used by metrics_test as well as
// connection_test. If there are more consumers in the future, we can consider
// moving it to a new, standalone package.
func VerifyMetricsMatch(expectedMetrics, actualMetrics string, metricToIgnore string) error {
	gotScanner := bufio.NewScanner(strings.NewReader(strings.TrimSpace(actualMetrics)))
	wantScanner := bufio.NewScanner(strings.NewReader(strings.TrimSpace(expectedMetrics)))
	for gotScanner.Scan() {
		wantScanner.Scan()
		wantLine := strings.TrimSpace(wantScanner.Text())
		gotLine := strings.TrimSpace(gotScanner.Text())
		if wantLine != gotLine && (metricToIgnore == "" || !strings.HasPrefix(gotLine, metricToIgnore)) {
			return fmt.Errorf("\r\nMetric Want: %q\r\nMetric Got:  %q\r\n", wantLine, gotLine)
		}
	}

	return nil
}

func (cmm *csiMetricsManager) registerMetrics() {
	cmm.registry.MustRegister(cmm.csiOperationsLatencyMetric)
}

func getErrorCode(err error) string {
	if err == nil {
		return codes.OK.String()
	}

	st, ok := status.FromError(err)
	if !ok {
		// This is not gRPC error. The operation must have failed before gRPC
		// method was called, otherwise we would get gRPC error.
		return "unknown-non-grpc"
	}

	return st.Code().String()
}
