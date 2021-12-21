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

	"github.com/csi-addons/spec/lib/go/reclaimspace"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ControllerReclaimSpace executes the ControllerReclaimSpace operation.
type ControllerReclaimSpace struct {
	// inherit Connect() and Close() from type grpcClient
	grpcClient

	persistentVolume string
}

var _ = registerOperation("ControllerReclaimSpace", &ControllerReclaimSpace{})

func (crs *ControllerReclaimSpace) Init(c *command) error {
	crs.persistentVolume = c.persistentVolume

	if crs.persistentVolume == "" {
		return fmt.Errorf("persistentvolume name is not set")
	}

	return nil
}

func (crs *ControllerReclaimSpace) Execute() error {
	c := getKubernetesClient()

	pv, err := c.CoreV1().PersistentVolumes().Get(context.TODO(), crs.persistentVolume, metav1.GetOptions{})
	if err != nil {
		return err
	}

	volID := pv.Spec.CSI.VolumeHandle
	volattributes := pv.Spec.CSI.VolumeAttributes

	deviceSecret, err := getSecret(c,
		pv.Spec.CSI.NodeStageSecretRef.Namespace,
		pv.Spec.CSI.NodeStageSecretRef.Name)
	if err != nil {
		return err
	}

	csiReq := &reclaimspace.ControllerReclaimSpaceRequest{
		VolumeId:   volID,
		Parameters: volattributes,
		Secrets:    deviceSecret,
	}

	service := reclaimspace.NewReclaimSpaceControllerClient(crs.Client)
	res, err := service.ControllerReclaimSpace(context.TODO(), csiReq)
	if err != nil {
		return err
	}

	fmt.Printf("space reclaimed for %q: %+v\n", crs.persistentVolume, res)

	return nil
}

// NodeReclaimSpace executes the NodeReclaimSpace operation.
type NodeReclaimSpace struct {
	// inherit Connect() and Close() from type grpcClient
	grpcClient

	persistentVolume  string
	stagingTargetPath string
}

var _ = registerOperation("NodeReclaimSpace", &NodeReclaimSpace{})

func (nrs *NodeReclaimSpace) Init(c *command) error {
	nrs.persistentVolume = c.persistentVolume
	if nrs.persistentVolume == "" {
		return fmt.Errorf("persistentvolume name is not set")
	}

	if c.stagingPath == "" {
		return fmt.Errorf("stagingpath is not set")
	}
	nrs.stagingTargetPath = fmt.Sprintf("%s/%s/globalmount", c.stagingPath, c.persistentVolume)

	return nil
}

func (nrs *NodeReclaimSpace) Execute() error {
	c := getKubernetesClient()

	pv, err := c.CoreV1().PersistentVolumes().Get(context.TODO(), nrs.persistentVolume, metav1.GetOptions{})
	if err != nil {
		return err
	}

	volID := pv.Spec.CSI.VolumeHandle

	deviceSecret, err := getSecret(c,
		pv.Spec.CSI.NodeStageSecretRef.Namespace,
		pv.Spec.CSI.NodeStageSecretRef.Name)
	if err != nil {
		return err
	}

	csiReq := &reclaimspace.NodeReclaimSpaceRequest{
		VolumeId:          volID,
		VolumeCapability:  nil,
		StagingTargetPath: nrs.stagingTargetPath,
		Secrets:           deviceSecret,
	}

	service := reclaimspace.NewReclaimSpaceNodeClient(nrs.Client)
	res, err := service.NodeReclaimSpace(context.TODO(), csiReq)
	if err != nil {
		return err
	}

	fmt.Printf("space reclaimed for %q: %+v\n", nrs.persistentVolume, res)

	return nil
}
