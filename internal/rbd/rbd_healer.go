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
	"fmt"
	"path/filepath"
	"sync"

	"github.com/ceph/ceph-csi/internal/util"
	kubeclient "github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
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
func getSecret(c *k8s.Clientset, ns, name string) (map[string]string, error) {
	deviceSecret := make(map[string]string)

	secret, err := c.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.ErrorLogMsg("get secret failed, err: %v", err)

		return nil, err
	}

	for k, v := range secret.Data {
		deviceSecret[k] = string(v)
	}

	return deviceSecret, nil
}

// formatStagingTargetPath returns the path where the volume is expected to be
// mounted (or the block-device is attached/mapped). Different Kubernetes
// version use different paths.
func formatStagingTargetPath(c *k8s.Clientset, pv *v1.PersistentVolume, stagingPath string) (string, error) {
	// Kubernetes 1.24+ uses a hash of the volume-id in the path name
	unique := sha256.Sum256([]byte(pv.Spec.CSI.VolumeHandle))
	targetPath := filepath.Join(stagingPath, pv.Spec.CSI.Driver, fmt.Sprintf("%x", unique), "globalmount")

	major, minor, err := kubeclient.GetServerVersion(c)
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	// 'encode' major/minor in a single integer
	legacyVersion := 1024 // Kubernetes 1.24 => 1 * 1000 + 24
	if ((major * 1000) + minor) < (legacyVersion) {
		// path in Kubernetes < 1.24
		targetPath = filepath.Join(stagingPath, "pv", pv.Name, "globalmount")
	}

	return targetPath, nil
}

func callNodeStageVolume(ns *NodeServer, c *k8s.Clientset, pv *v1.PersistentVolume, stagingPath string) error {
	publishContext := make(map[string]string)

	volID := pv.Spec.PersistentVolumeSource.CSI.VolumeHandle
	stagingParentPath, err := formatStagingTargetPath(c, pv, stagingPath)
	if err != nil {
		log.ErrorLogMsg("formatStagingTargetPath failed volID: %s, err: %v", volID, err)

		return err
	}

	log.DefaultLog("sending nodeStageVolume for volID: %s, stagingPath: %s",
		volID, stagingParentPath)

	deviceSecret, err := getSecret(c,
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
func RunVolumeHealer(ns *NodeServer, conf *util.Config) error {
	c, err := kubeclient.NewK8sClient()
	if err != nil {
		log.ErrorLogMsg("failed to connect to Kubernetes: %v", err)

		return err
	}

	val, err := c.StorageV1().VolumeAttachments().List(context.TODO(), metav1.ListOptions{})
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
		pv, err := c.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		if err != nil {
			// skip if volume doesn't exist
			if !apierrors.IsNotFound(err) {
				log.ErrorLogMsg("get persistentVolumes failed for pv: %s, err: %v", pvName, err)
			}

			continue
		}
		// skip this volumeattachment if its pv is not bound or marked for deletion
		if pv.Status.Phase != v1.VolumeBound || pv.DeletionTimestamp != nil {
			continue
		}
		// skip if mounter is not rbd-nbd
		if pv.Spec.PersistentVolumeSource.CSI.VolumeAttributes["mounter"] != "rbd-nbd" {
			continue
		}

		// ensure that the volume is still in attached state
		va, err := c.StorageV1().VolumeAttachments().Get(context.TODO(), val.Items[i].Name, metav1.GetOptions{})
		if err != nil {
			// skip if volume attachment doesn't exist
			if !apierrors.IsNotFound(err) {
				log.ErrorLogMsg("get volumeAttachments failed for volumeAttachment: %s, volID: %s, err: %v",
					val.Items[i].Name, pv.Spec.PersistentVolumeSource.CSI.VolumeHandle, err)
			}

			continue
		}
		if !va.Status.Attached {
			continue
		}

		wg.Add(1)
		// run multiple NodeStageVolume calls concurrently
		go func(wg *sync.WaitGroup, ns *NodeServer, c *k8s.Clientset, pv *v1.PersistentVolume, stagingPath string) {
			defer wg.Done()
			channel <- callNodeStageVolume(ns, c, pv, stagingPath)
		}(&wg, ns, c, pv, conf.StagingPath)
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
