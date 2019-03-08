/*
Copyright 2019 ceph-csi authors.

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

package util

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
)

/*
K8sConfig is a ConfigStore interface implementation that reads configuration
information from k8s secrets.

Each Ceph cluster configuration secret is expected to be named,
ceph-cluster-<fsid>, where <fsid> is the Ceph cluster fsid.

The secret is expected to contain keys, as defined by the ConfigKeys constants
in the ConfigStore interface.
*/
type K8sConfig struct {
	Client    *k8s.Clientset
	Namespace string
}

// DataForKey reads the appropriate k8s secret, named using fsid, and returns
// the contents of key within the secret
func (kc *K8sConfig) DataForKey(fsid string, key string) (data string, err error) {
	secret, err := kc.Client.CoreV1().Secrets(kc.Namespace).Get("ceph-cluster-"+fsid, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("error fetching configuration for cluster ID (%s). (%s)", fsid, err)
		return
	}

	content, ok := secret.Data[key]
	if !ok {
		err = fmt.Errorf("missing data for key (%s) in cluster configuration of (%s)", key, fsid)
		return
	}

	data = string(content)
	return
}
