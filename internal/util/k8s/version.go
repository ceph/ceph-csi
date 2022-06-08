/*
Copyright 2022 The Ceph-CSI Authors.

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

package k8s

import (
	"fmt"
	"strconv"

	"k8s.io/client-go/kubernetes"
)

// GetServerVersion returns kubernetes server major and minor version as
// integer.
func GetServerVersion(client *kubernetes.Clientset) (int, int, error) {
	version, err := client.ServerVersion()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get ServerVersion: %w", err)
	}

	major, err := strconv.Atoi(version.Major)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert Kubernetes major version %q to int: %w", version.Major, err)
	}

	minor, err := strconv.Atoi(version.Minor)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert Kubernetes minor version %q to int: %w", version.Minor, err)
	}

	return major, minor, nil
}
