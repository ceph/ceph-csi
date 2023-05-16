/*
Copyright 2023 The Ceph-CSI Authors.

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

package cephfs

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/ghodss/yaml"
	v1 "k8s.io/api/core/v1"
)

//go:embed csi-config-map.yaml
var csiConfigMap string

type CSIConfigMapValues struct {
	Name string
}

var CSIConfigMapDefaults = CSIConfigMapValues{
	Name: "ceph-csi-config",
}

// NewCSIConfigMap takes a name from the CSIConfigMapValues struct and relaces
// the value in the template. A ConfigMap object is returned which can be
// created in the Kubernetes cluster.
func NewCSIConfigMap(values CSIConfigMapValues) (*v1.ConfigMap, error) {
	data, err := NewCSIConfigMapYAML(values)
	if err != nil {
		return nil, err
	}

	cm := &v1.ConfigMap{}
	err = yaml.Unmarshal([]byte(data), cm)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", cm, err)
	}

	return cm, nil
}

// NewCSIConfigMapYAML takes a name from the CSIConfigMapValues struct and
// relaces the value in the template. A ConfigMap object in YAML is returned
// which can be created in the Kubernetes cluster.
func NewCSIConfigMapYAML(values CSIConfigMapValues) (string, error) {
	var buf bytes.Buffer

	tmpl, err := template.New("CSIConfigMap").Parse(csiConfigMap)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	err = tmpl.Execute(&buf, values)
	if err != nil {
		return "", fmt.Errorf("failed to replace values in template: %w", err)
	}

	return buf.String(), nil
}
