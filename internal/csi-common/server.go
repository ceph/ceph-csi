/*
Copyright 2017 The Kubernetes Authors.

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
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/csi-addons/spec/lib/go/replication"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// NonBlockingGRPCServer defines Non blocking GRPC server interfaces.
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint, hstOptions string, srv Servers, metrics bool)
	// Waits for the service to stop
	Wait()
	// Stops the service gracefully
	Stop()
	// Stops the service forcefully
	ForceStop()
}

// Servers holds the list of servers.
type Servers struct {
	IS csi.IdentityServer
	CS csi.ControllerServer
	NS csi.NodeServer
	RS replication.ControllerServer
}

// NewNonBlockingGRPCServer return non-blocking GRPC.
func NewNonBlockingGRPCServer() NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// NonBlocking server.
type nonBlockingGRPCServer struct {
	wg     sync.WaitGroup
	server *grpc.Server
}

// Start start service on endpoint.
func (s *nonBlockingGRPCServer) Start(endpoint, hstOptions string, srv Servers, metrics bool) {
	s.wg.Add(1)
	go s.serve(endpoint, hstOptions, srv, metrics)
}

// Wait blocks until the WaitGroup counter.
func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

// GracefulStop stops the gRPC server gracefully.
func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
}

// Stop stops the gRPC server.
func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(endpoint, hstOptions string, srv Servers, metrics bool) {
	proto, addr, err := parseEndpoint(endpoint)
	if err != nil {
		klog.Fatal(err.Error())
	}

	if proto == "unix" {
		addr = "/" + addr
		if e := os.Remove(addr); e != nil && !os.IsNotExist(e) {
			klog.Fatalf("Failed to remove %s, error: %s", addr, e.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		klog.Fatalf("Failed to listen: %v", err)
	}

	opts := []grpc.ServerOption{
		NewMiddlewareServerOption(metrics),
	}

	server := grpc.NewServer(opts...)
	s.server = server

	if srv.IS != nil {
		csi.RegisterIdentityServer(server, srv.IS)
	}
	if srv.CS != nil {
		csi.RegisterControllerServer(server, srv.CS)
	}
	if srv.NS != nil {
		csi.RegisterNodeServer(server, srv.NS)
	}
	if srv.RS != nil {
		replication.RegisterControllerServer(server, srv.RS)
	}

	log.DefaultLog("Listening for connections on address: %#v", listener.Addr())
	if metrics {
		ho := strings.Split(hstOptions, ",")
		const expectedHo = 3
		if len(ho) != expectedHo {
			klog.Fatalf("invalid histogram options provided: %v", hstOptions)
		}
		start, e := strconv.ParseFloat(ho[0], 32)
		if e != nil {
			klog.Fatalf("failed to parse histogram start value: %v", e)
		}
		factor, e := strconv.ParseFloat(ho[1], 32)
		if err != nil {
			klog.Fatalf("failed to parse histogram factor value: %v", e)
		}
		count, e := strconv.Atoi(ho[2])
		if err != nil {
			klog.Fatalf("failed to parse histogram count value: %v", e)
		}
		buckets := prometheus.ExponentialBuckets(start, factor, count)
		bktOptions := grpc_prometheus.WithHistogramBuckets(buckets)
		grpc_prometheus.EnableHandlingTimeHistogram(bktOptions)
		grpc_prometheus.Register(server)
	}
	err = server.Serve(listener)
	if err != nil {
		klog.Fatalf("Failed to server: %v", err)
	}
}
