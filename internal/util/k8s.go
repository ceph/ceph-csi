/*
Copyright 2020 The CephCSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"os"

	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"
)

// NewK8sClient create kubernetes client.
func NewK8sClient() *k8s.Clientset {
	var cfg *rest.Config
	var err error
	cPath := os.Getenv("KUBERNETES_CONFIG_PATH")
	if cPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", cPath)
		if err != nil {
			klog.Errorf("Failed to get cluster config with error: %v\n", err)
			os.Exit(1)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			klog.Errorf("Failed to get cluster config with error: %v\n", err)
			os.Exit(1)
		}
	}
	client, err := k8s.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("Failed to create client with error: %v\n", err)
		os.Exit(1)
	}
	return client
}
