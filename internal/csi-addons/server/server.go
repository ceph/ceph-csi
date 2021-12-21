/*
Copyright 2021 The Ceph-CSI Authors.

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

package server

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"

	"google.golang.org/grpc"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util/log"
)

var ErrNoUDS = errors.New("no UNIX domain socket")

// CSIAddonsService is the interface that is required to be implemented so that
// the CSIAddonsServer can register the service by calling RegisterService().
type CSIAddonsService interface {
	// RegisterService is called by the CSIAddonsServer to add a CSI-Addons
	// service that can handle requests.
	RegisterService(server grpc.ServiceRegistrar)
}

// CSIAddonsServer is the gRPC server that listens on an endpoint (UNIX domain
// socket) where the CSI-Addons requests come in.
type CSIAddonsServer struct {
	// URL components to listen on the UNIX domain socket
	scheme string
	path   string

	// state of the CSIAddonsServer
	server   *grpc.Server
	services []CSIAddonsService
}

// NewCSIAddonsServer create a new CSIAddonsServer on the given endpoint. The
// endpoint should be a URL. Only UNIX domain sockets are supported.
func NewCSIAddonsServer(endpoint string) (*CSIAddonsServer, error) {
	cas := &CSIAddonsServer{}

	if cas.services == nil {
		cas.services = make([]CSIAddonsService, 0)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "unix" {
		return nil, fmt.Errorf("%w: %s", ErrNoUDS, endpoint)
	}

	cas.scheme = u.Scheme
	cas.path = u.Path

	return cas, nil
}

// RegisterService takes the CSIAddonsService and registers it with the
// CSIAddonsServer gRPC server. This function should be called before Start,
// where the services are registered on the internal gRPC server.
func (cas *CSIAddonsServer) RegisterService(svc CSIAddonsService) {
	cas.services = append(cas.services, svc)
}

// Start creates the internal gRPC server, and registers the CSIAddonsServices.
// The internal gRPC server is started in it's own go-routine when no error is
// returned.
func (cas *CSIAddonsServer) Start() error {
	// create the gRPC server and register services
	cas.server = grpc.NewServer(csicommon.NewMiddlewareServerOption(false))

	for _, svc := range cas.services {
		svc.RegisterService(cas.server)
	}

	// setup the UNIX domain socket
	if e := os.Remove(cas.path); e != nil && !os.IsNotExist(e) {
		return fmt.Errorf("failed to remove %q: %w", cas.path, e)
	}

	listener, err := net.Listen(cas.scheme, cas.path)
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %w", cas.path, err)
	}

	go cas.serve(listener)

	return nil
}

// serve starts the actual process of listening for requests on the gRPC
// server. This is a blocking call, so it should get executed in a go-routine.
func (cas *CSIAddonsServer) serve(listener net.Listener) {
	log.DefaultLog("listening for CSI-Addons requests on address: %#v", listener.Addr())

	// start to serve requests
	err := cas.server.Serve(listener)
	if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.FatalLogMsg("failed to setup CSI-Addons server: %v", err)
	}

	log.DefaultLog("the CSI-Addons server at %q has been stopped", listener.Addr())
}

// Stop can be used to stop the internal gRPC server.
func (cas *CSIAddonsServer) Stop() {
	if cas.server == nil {
		return
	}

	cas.server.GracefulStop()
}
