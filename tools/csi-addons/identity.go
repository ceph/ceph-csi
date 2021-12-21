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

package main

import (
	"context"
	"fmt"

	"github.com/csi-addons/spec/lib/go/identity"
)

// GetIdentity executes the GetIdentity operation.
type GetIdentity struct {
	// inherit Connect() and Close() from type grpcClient
	grpcClient
}

var _ = registerOperation("GetIdentity", &GetIdentity{})

func (gi *GetIdentity) Init(c *command) error {
	return nil
}

func (gi *GetIdentity) Execute() error {
	service := identity.NewIdentityClient(gi.Client)
	res, err := service.GetIdentity(context.TODO(), &identity.GetIdentityRequest{})
	if err != nil {
		return err
	}

	fmt.Printf("identity: %+v\n", res)

	return nil
}

// Probe executes the Probe operation.
type Probe struct {
	// inherit Connect() and Close() from type grpcClient
	grpcClient
}

var _ = registerOperation("Probe", &Probe{})

func (p *Probe) Init(c *command) error {
	return nil
}

func (p *Probe) Execute() error {
	service := identity.NewIdentityClient(p.Client)
	res, err := service.Probe(context.TODO(), &identity.ProbeRequest{})
	if err != nil {
		return err
	}

	fmt.Printf("probe succeeded: %+v\n", res)

	return nil
}
