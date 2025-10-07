/*
Copyright 2025 The Ceph-CSI Authors.

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

package e2e

import (
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubectl "k8s.io/kubernetes/test/e2e/framework/kubectl"
)

// detectOpenShift checks for the clusterversion that is provided by OpenShift.
// If the clusterversion is not found, we're not running on an OpenShift
// cluster.
func detectOpenShift() (bool, error) {
	version, err := e2ekubectl.RunKubectl("openshift",
		"get",
		"clusterversion.config.openshift.io/version",
		"-ojsonpath='{.status.desired.version}'",
	)

	if err != nil {
		if isNoSuchResourceCLIError(err) {
			return false, nil
		}

		return false, err
	}

	framework.Logf("running on OpenShift version %s", version)

	return true, nil
}
