/*
Copyright 2020 The Ceph-CSI Authors.

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

package liveness

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ceph/ceph-csi/internal/cephfs"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var secrets = make(map[string]*util.Credentials)

var (
	rbdMapping = *prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "csi",
		Name:      "ceph_rbd",
		Help:      "ceph rbd image pvc mapping"},
		[]string{"pvc_name", "pvc_namespace", "pv_name", "pool", "imageID"})
	cephfsMapping = *prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "csi",
		Name:      "ceph_fs",
		Help:      "cephfs subvolume pvc mapping"},
		[]string{"pvc_name", "pvc_namespace", "pv_name", "volume_name", "subvolume_group", "subvolume_name"})
)

const (
	// csiConfigFile is the location of the CSI config file
	csiConfigFile = "/etc/ceph-csi-config/config.json"
)

func getMapping(timeout time.Duration, client *k8s.Clientset, numGoroutines int, driverName string) {
	util.DebugLogMsg("Start recording mapping")
	pvcs, err := client.CoreV1().PersistentVolumeClaims("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Get pvclist failed: %v", err)
		return
	}
	var relatedPvcs []v1.PersistentVolumeClaim
	var length = 0
	for index := range pvcs.Items {
		var related bool = false
		for key, value := range pvcs.Items[index].Annotations {
			if strings.Contains(key, "storage-provisioner") && value == driverName {
				related = true
			}
		}
		if !related {
			continue
		}
		relatedPvcs = append(relatedPvcs, pvcs.Items[index])
		length++
	}
	var wg sync.WaitGroup
	jobs := make(chan v1.PersistentVolumeClaim, numGoroutines)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*timeout)
	defer cancel()
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go getPvcMappings(ctx, client, jobs, &wg)
	}
	for index := range relatedPvcs {
		jobs <- relatedPvcs[index]
	}
	close(jobs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
		util.DebugLogMsg("Collect mapping metrics complete")
	case <-time.After(timeout):
		klog.Errorf("Collect mapping metrics timeout")
	}
}

func collectPvcMapping(client *k8s.Clientset, pvc *v1.PersistentVolumeClaim, secrets map[string]*util.Credentials) {
	// get pv info
	pvName := pvc.Spec.VolumeName
	if pvName == "" {
		return
	}
	pvcName := pvc.Name
	pvcNameSpace := pvc.Namespace
	pvInfo, err := client.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Get pvinfo failed: %v", err)
		return
	}
	if pvInfo.Spec.CSI == nil {
		return
	}

	// splicing image name
	volumeNamePrefix := pvInfo.Spec.CSI.VolumeAttributes["volumeNamePrefix"]
	if volumeNamePrefix == "" {
		volumeNamePrefix = "csi-vol"
	}
	volumeHandle := pvInfo.Spec.CSI.VolumeHandle
	volumeHandleSlice := strings.Split(volumeHandle, "-")
	ImageIDLength := 5
	index := len(volumeHandleSlice) - ImageIDLength
	imageName := strings.Join(append([]string{volumeNamePrefix}, volumeHandleSlice[index:]...), "-")
	var secretName string
	var secretNamespace string

	// if ControllerExpandSecretRef is nil , get NodePublishSecretRef to compatible with pv created by csi lower than v1.2
	if pvInfo.Spec.CSI.ControllerExpandSecretRef != nil {
		secretName = pvInfo.Spec.CSI.ControllerExpandSecretRef.Name
		secretNamespace = pvInfo.Spec.CSI.ControllerExpandSecretRef.Namespace
	} else {
		secretName = pvInfo.Spec.CSI.NodePublishSecretRef.Name
		secretNamespace = pvInfo.Spec.CSI.NodePublishSecretRef.Namespace
	}
	imageCluster := pvInfo.Spec.CSI.VolumeAttributes["clusterID"]
	monitors, err := util.Mons(csiConfigFile, imageCluster)
	if err != nil {
		klog.Errorf(err.Error())
		return
	}
	radosNamespace, err := util.RadosNamespace(csiConfigFile, imageCluster)
	if err != nil {
		klog.Errorf(err.Error())
		return
	}

	// get secret
	var cr *util.Credentials
	if value, exist := secrets[secretNamespace+"/"+secretName]; exist {
		cr = value
	} else {
		var secret *v1.Secret
		secret, err = client.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf(err.Error())
			return
		}
		newSecrets := make(map[string]string)
		for key, value := range secret.Data {
			newSecrets[key] = string(value)
		}
		if _, ok := newSecrets["adminID"]; ok {
			cr, err = util.NewAdminCredentials(newSecrets)
		} else {
			cr, err = util.NewUserCredentials(newSecrets)
		}
		if err != nil {
			klog.Errorf(err.Error())
			return
		}
		secrets[secretNamespace+"/"+secretName] = cr
	}

	imagePool := pvInfo.Spec.CSI.VolumeAttributes["pool"]
	volumeName := pvInfo.Spec.CSI.VolumeAttributes["fsName"]
	// if backend image/subvolume exist,  set metrics to 1 else set 0
	switch {
	case imagePool == "" && volumeName == "":
		// unknown volume
		klog.Warningf("Found unknown type pvc %s with pvName %s", pvcName, pvName)
		return
	case volumeName != "":
		// cephfs volume
		subvolumeGroup, err := util.CephFSSubvolumeGroup(csiConfigFile, imageCluster)
		if err != nil {
			klog.Errorf("Failed to get subvolumeGroup: %s", err.Error())
			return
		}
		if cephfs.CheckSubVolumeInCluster(monitors, imageName, subvolumeGroup, volumeName, cr) {
			cephfsMapping.WithLabelValues(pvcName, pvcNameSpace, pvName, volumeName, subvolumeGroup, imageName).Set(1)
		} else {
			cephfsMapping.WithLabelValues(pvcName, pvcNameSpace, pvName, volumeName, subvolumeGroup, imageName).Set(0)
		}
	default:
		// rbd volume
		err := rbd.GetImageInfo(context.TODO(), monitors, cr, imagePool, imageName, radosNamespace)
		if err != nil {
			rbdMapping.WithLabelValues(pvcName, pvcNameSpace, pvName, imagePool, imageName).Set(0)
			util.ExtendedLogMsg(err.Error())
		} else {
			rbdMapping.WithLabelValues(pvcName, pvcNameSpace, pvName, imagePool, imageName).Set(1)
		}
	}
}

func getPvcMappings(ctx context.Context, client *k8s.Clientset, pvcs <-chan v1.PersistentVolumeClaim, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case pvc, ok := <-pvcs:
			if !ok {
				return
			}
			collectPvcMapping(client, &pvc, secrets)
		case <-ctx.Done():
			klog.Errorf("Collect mapping metrics timeout , you can increase scrapeMetricsJobs or scrapeMetricsTimeout")
			return
		}
	}
}

func recordMapping(pollTime, pollTimeout time.Duration, csiConn *grpc.ClientConn, scapeTimeout time.Duration, numGoroutines int) {
	// register prometheus metrics
	vType := os.Getenv("DRIVER_TYPE")
	switch {
	case vType == "rbd":
		err := prometheus.Register(rbdMapping)
		if err != nil {
			klog.Fatalln(err)
		}
	case vType == "cephfs":
		err := prometheus.Register(cephfsMapping)
		if err != nil {
			klog.Fatalln(err)
		}
	default:
		err := fmt.Errorf("environment variables DRIVER_TYPE should be set to rbd or cephfs")
		klog.Fatalln(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	defer cancel()
	driverName, err := rpc.GetDriverName(ctx, csiConn)
	if err != nil {
		klog.Fatalf("failed to driver name: %v", err)
	}
	// get mappings periodically
	client := util.NewK8sClient()
	ticker := time.NewTicker(pollTime)
	defer ticker.Stop()
	for range ticker.C {
		getMapping(scapeTimeout, client, numGoroutines, driverName)
	}
}
