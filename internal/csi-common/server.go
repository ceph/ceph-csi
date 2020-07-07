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

	"github.com/container-storage-interface/spec/lib/go/csi"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

// NonBlockingGRPCServer defines Non blocking GRPC server interfaces.
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint, hstOptions string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, metrics bool)
	// Waits for the service to stop
	Wait()
	// Stops the service gracefully
	Stop()
	// Stops the service forcefully
	ForceStop()
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
func (s *nonBlockingGRPCServer) Start(endpoint, hstOptions string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, metrics bool) {
	s.wg.Add(1)
	go s.serve(endpoint, hstOptions, ids, cs, ns, metrics)
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

func (s *nonBlockingGRPCServer) serve(endpoint, hstOptions string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, metrics bool) {
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

	middleWare := []grpc.UnaryServerInterceptor{contextIDInjector, logGRPC, panicHandler}
	if metrics {
		middleWare = append(middleWare, grpc_prometheus.UnaryServerInterceptor)
	}
	opts := []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(middleWare...),
	}

	server := grpc.NewServer(opts...)
	s.server = server

	if ids != nil {
		csi.RegisterIdentityServer(server, ids)
	}
	if cs != nil {
		csi.RegisterControllerServer(server, cs)
	}
	if ns != nil {
		csi.RegisterNodeServer(server, ns)
	}
	klog.V(1).Infof("Listening for connections on address: %#v", listener.Addr()) // nolint:gomnd // number specifies log level
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
		bktOptios := grpc_prometheus.WithHistogramBuckets(buckets)
		grpc_prometheus.EnableHandlingTimeHistogram(bktOptios)
		grpc_prometheus.Register(server)
	}
	err = server.Serve(listener)
	if err != nil {
		klog.Fatalf("Failed to server: %v", err)
	}
}
