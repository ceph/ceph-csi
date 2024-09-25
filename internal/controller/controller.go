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
package controller

import (
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	clientConfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Manager is the interface that will wrap Add function.
// The New controllers which gets added, as to implement Add function to get
// started by the manager.
type Manager interface {
	Add(mgr manager.Manager, cfg Config) error
}

// Config holds the drivername and namespace name.
type Config struct {
	DriverName  string
	Namespace   string
	ClusterName string
	InstanceID  string
	SetMetadata bool
}

// ControllerList holds the list of managers need to be started.
var ControllerList []Manager

// addToManager calls the registered managers Add method.
func addToManager(mgr manager.Manager, config Config) error {
	for _, c := range ControllerList {
		err := c.Add(mgr, config)
		if err != nil {
			return fmt.Errorf("failed to add: %w", err)
		}
	}

	return nil
}

// Start will start all the registered managers.
func Start(config Config) error {
	electionID := config.DriverName + "-" + config.Namespace
	opts := manager.Options{
		LeaderElection: true,
		// disable metrics
		Metrics:                    metricsserver.Options{BindAddress: "0"},
		LeaderElectionNamespace:    config.Namespace,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionID:           electionID,
	}

	kubeConfig := clientConfig.GetConfigOrDie()
	coreKubeConfig := rest.CopyConfig(kubeConfig)
	coreKubeConfig.ContentType = runtime.ContentTypeProtobuf
	mgr, err := manager.New(coreKubeConfig, opts)
	if err != nil {
		log.ErrorLogMsg("failed to create manager %s", err)

		return err
	}
	err = addToManager(mgr, config)
	if err != nil {
		log.ErrorLogMsg("failed to add manager %s", err)

		return err
	}
	err = mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		log.ErrorLogMsg("failed to start manager %s", err)
	}

	return err
}
