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
// source https://github.com/kubernetes-csi/cluster-driver-registrar/blob/master/cmd/csi-cluster-driver-registrar/k8s_register.go
package util

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	k8scsi "k8s.io/api/storage/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
)

// createOrUpdateCSIDriverInfo Registers CSI driver by creating a CSIDriver object
func createOrUpdateCSIDriverInfo(csiClientset *kubernetes.Clientset,
	name string) error {
	attach := false
	mountInfo := true
	// Create CSIDriver object
	csiDriver := &k8scsi.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: k8scsi.CSIDriverSpec{
			AttachRequired: &attach,
			PodInfoOnMount: &mountInfo,
		},
	}
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		csidrivers := csiClientset.StorageV1beta1().CSIDrivers()

		_, err := csidrivers.Create(csiDriver)
		if err == nil {
			klog.V(2).Infof("CSIDriver object created for driver %s", csiDriver.Name)
			return nil
		} else if apierrors.IsAlreadyExists(err) {
			klog.V(2).Info("CSIDriver CRD already had been registered")
			return nil
		}
		klog.Errorf("failed to create CSIDriver object: %v", err)
		return err
	})
	return retryErr
}

func cleanup(c <-chan os.Signal, d chan bool, clientSet *kubernetes.Clientset, name string) {
	<-c
	d <- true
	err := deleteCSIDriverInfo(clientSet, name)
	if err != nil {
		klog.Errorf("failed to delete CSIDriver object: %v", err)
		os.Exit(1)
	}
}

// Deregister CSI Driver by deleting CSIDriver object
func deleteCSIDriverInfo(csiClientset *kubernetes.Clientset, name string) error {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		csidrivers := csiClientset.StorageV1beta1().CSIDrivers()
		err := csidrivers.Delete(name, &metav1.DeleteOptions{})
		if err == nil {
			klog.V(2).Infof("CSIDriver object deleted for driver %s", name)
			return nil
		} else if apierrors.IsNotFound(err) {
			klog.V(2).Info("no need to clean up CSIDriver since it does not exist")
			return nil
		}
		klog.Errorf("failed to delete CSIDriver object: %v", err)
		return err
	})
	return retryErr
}

// RegisterCSIDriver create CSIDriver object during the controller plugin
// starts and delete the CSIDriver object when controller plugin terminates
func RegisterCSIDriver(c chan os.Signal, name string) {
	k8sClient := NewK8sClient()

	signal.Notify(c, syscall.SIGTERM)

	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)

	// Set up goroutine to cleanup (aka deregister) on termination.
	go cleanup(c, done, k8sClient, name)

	err := createOrUpdateCSIDriverInfo(k8sClient, name)
	if err != nil {
		klog.Fatalf("failed to create CSIDriver object for %q: %v\n", name, err)
	}
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// update the CSI driver object if it is deleted
			err = createOrUpdateCSIDriverInfo(k8sClient, name)
			if err != nil {
				klog.Fatalf("failed to create CSIDriver object for %q: %v\n", name, err)
			}
		}

	}

}
