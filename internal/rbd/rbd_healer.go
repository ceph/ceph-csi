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

package rbd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	v1 "k8s.io/api/core/v1"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	fsTypeBlockName = "block"
)

// accessModeStrToInt convert access mode type string to int32.
// Make sure to update this function as and when there are new modes introduced.
func accessModeStrToInt(mode v1.PersistentVolumeAccessMode) csi.VolumeCapability_AccessMode_Mode {
	switch mode {
	case v1.ReadWriteOnce:
		return csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER
	case v1.ReadOnlyMany:
		return csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY
	case v1.ReadWriteMany:
		return csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER
	case v1.ReadWriteOncePod:
		return csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER
	}

	return csi.VolumeCapability_AccessMode_UNKNOWN
}

// getSecret get the secret details by name.
func getSecret(ns, name string) (map[string]string, error) {
	deviceSecret := make(map[string]string)

	secretData, err := k8s.GetSecret(name, ns)
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w", ns, name, err)
	}

	for k, v := range secretData {
		deviceSecret[k] = v
	}

	return deviceSecret, nil
}

// formatStagingTargetPath returns the path where the volume is expected to be
// mounted (or the block-device is attached/mapped). Different Kubernetes
// version use different paths.
func formatStagingTargetPath(pv *v1.PersistentVolume, stagingPath string) string {
	// Kubernetes 1.24+ uses a hash of the volume-id in the path name
	unique := sha256.Sum256([]byte(pv.Spec.CSI.VolumeHandle))
	targetPath := filepath.Join(stagingPath, pv.Spec.CSI.Driver, hex.EncodeToString(unique[:]), "globalmount")

	return targetPath
}

func (ns *NodeServer) callNodeStageVolume(pv *v1.PersistentVolume, stagingPath string) error {
	publishContext := make(map[string]string)

	volID := pv.Spec.PersistentVolumeSource.CSI.VolumeHandle
	stagingParentPath := formatStagingTargetPath(pv, stagingPath)

	log.DefaultLog("sending nodeStageVolume for volID: %s, stagingPath: %s",
		volID, stagingParentPath)

	deviceSecret, err := getSecret(
		pv.Spec.PersistentVolumeSource.CSI.NodeStageSecretRef.Namespace,
		pv.Spec.PersistentVolumeSource.CSI.NodeStageSecretRef.Name)
	if err != nil {
		log.ErrorLogMsg("getSecret failed for volID: %s, err: %v", volID, err)

		return err
	}

	volumeContext := pv.Spec.PersistentVolumeSource.CSI.VolumeAttributes
	volumeContext["volumeHealerContext"] = "true"

	req := &csi.NodeStageVolumeRequest{
		VolumeId:          volID,
		PublishContext:    publishContext,
		StagingTargetPath: stagingParentPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: accessModeStrToInt(pv.Spec.AccessModes[0]),
			},
		},
		Secrets:       deviceSecret,
		VolumeContext: volumeContext,
	}
	if pv.Spec.PersistentVolumeSource.CSI.FSType == fsTypeBlockName {
		req.VolumeCapability.AccessType = &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		}
	} else {
		req.VolumeCapability.AccessType = &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{
				FsType:     pv.Spec.PersistentVolumeSource.CSI.FSType,
				MountFlags: pv.Spec.MountOptions,
			},
		}
	}

	_, err = ns.NodeStageVolume(context.TODO(), req)
	if err != nil {
		log.ErrorLogMsg("nodeStageVolume request failed, volID: %s, stagingPath: %s, err: %v",
			volID, stagingParentPath, err)

		return err
	}

	return nil
}

// RunVolumeHealer heal the volumes attached on a node.
func (ns *NodeServer) RunVolumeHealer(conf *util.Config) error {
	val, err := k8s.GetVolumeAttachmentList()
	if err != nil {
		log.ErrorLogMsg("list volumeAttachments failed, err: %v", err)

		return err
	}

	var wg sync.WaitGroup
	channel := make(chan error)
	for i := range val.Items {
		// skip if the volumeattachments doesn't belong to current node or driver
		if val.Items[i].Spec.NodeName != conf.NodeID || val.Items[i].Spec.Attacher != conf.DriverName {
			continue
		}
		pvName := *val.Items[i].Spec.Source.PersistentVolumeName
		pv, err := k8s.GetPersistentVolume(pvName)
		if k8s.IgnoreNotFound(err) != nil {
			log.ErrorLogMsg("get persistentVolumes failed for pv: %s, err: %v", pvName, err)

			continue
		}

		// skip this volumeattachment if its pv is not bound or marked for deletion
		if pv == nil || pv.Status.Phase != v1.VolumeBound || pv.DeletionTimestamp != nil {
			continue
		}

		if pv.Spec.PersistentVolumeSource.CSI == nil {
			continue
		}
		// skip if mounter is not rbd-nbd
		if pv.Spec.PersistentVolumeSource.CSI.VolumeAttributes["mounter"] != "rbd-nbd" {
			continue
		}

		// ensure that the volume is still in attached state
		va, err := k8s.GetVolumeAttachment(val.Items[i].Name)
		if k8s.IgnoreNotFound(err) != nil {
			log.ErrorLogMsg("get volumeAttachments failed for volumeAttachment: %s, volID: %s, err: %v",
				val.Items[i].Name, pv.Spec.PersistentVolumeSource.CSI.VolumeHandle, err)

			continue
		}

		if va == nil || !va.Status.Attached {
			continue
		}

		wg.Add(1)
		// run multiple NodeStageVolume calls concurrently
		go func(wg *sync.WaitGroup, ns *NodeServer, pv *v1.PersistentVolume, stagingPath string) {
			defer wg.Done()
			channel <- ns.callNodeStageVolume(pv, stagingPath)
		}(&wg, ns, pv, conf.StagingPath)
	}

	go func() {
		wg.Wait()
		close(channel)
	}()

	for s := range channel {
		if s != nil {
			log.ErrorLogMsg("callNodeStageVolume failed, err: %v", s)
		}
	}

	return nil
}
