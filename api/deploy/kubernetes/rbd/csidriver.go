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
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/ghodss/yaml"
	storagev1 "k8s.io/api/storage/v1"
)

//go:embed csidriver.yaml
var csiDriver string

type CSIDriverValues struct {
	Name string
}

var CSIDriverDefaults = CSIDriverValues{
	Name: "rbd.csi.ceph.com",
}

// NewCSIDriver takes a driver name from the CSIDriverValues struct and relaces
// the value in the template. A CSIDriver object is returned which can be
// created in the Kubernetes cluster.
func NewCSIDriver(values CSIDriverValues) (*storagev1.CSIDriver, error) {
	data, err := NewCSIDriverYAML(values)
	if err != nil {
		return nil, err
	}

	driver := &storagev1.CSIDriver{}
	err = yaml.Unmarshal([]byte(data), driver)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", driver, err)
	}

	return driver, nil
}

// NewCSIDriverYAML takes a driver name from the CSIDriverValues struct and relaces
// the value in the template. A CSIDriver object in YAML is returned which can be
// created in the Kubernetes cluster.
func NewCSIDriverYAML(values CSIDriverValues) (string, error) {
	var buf bytes.Buffer

	tmpl, err := template.New("CSIDriver").Parse(csiDriver)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	err = tmpl.Execute(&buf, values)
	if err != nil {
		return "", fmt.Errorf("failed to replace values in template: %w", err)
	}

	return buf.String(), nil
}
