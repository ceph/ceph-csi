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
	"sync"

	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// NonBlockingGRPCServer defines Non blocking GRPC server interfaces.
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint string, srv Servers)
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
func (s *nonBlockingGRPCServer) Start(endpoint string, srv Servers) {
	s.wg.Add(1)
	go s.serve(endpoint, srv)
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

func (s *nonBlockingGRPCServer) serve(endpoint string, srv Servers) {
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

	server := grpc.NewServer(NewMiddlewareServerOption())
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

	log.DefaultLog("Listening for connections on address: %#v", listener.Addr())
	err = server.Serve(listener)
	if err != nil {
		klog.Fatalf("Failed to server: %v", err)
	}
}
