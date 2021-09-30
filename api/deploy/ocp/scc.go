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

package ocp

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/ghodss/yaml"
	secv1 "github.com/openshift/api/security/v1"
)

//go:embed scc.yaml
var securityContextConstraints string

// SecurityContextConstraintsValues contains values that need replacing in the
// template.
type SecurityContextConstraintsValues struct {
	// Namespace contains the OpenShift Namespace where the SCC will be
	// used.
	Namespace string
	// Deployer refers to the Operator that creates the SCC and
	// ServiceAccounts. This is an optional option.
	Deployer string
}

// SecurityContextConstraintsDefaults can be used for generating deployment
// artifacts with defails values.
var SecurityContextConstraintsDefaults = SecurityContextConstraintsValues{
	Namespace: "ceph-csi",
	Deployer:  "",
}

// NewSecurityContextConstraints creates a new SecurityContextConstraints
// object by replacing variables in the template by the values set in the
// SecurityContextConstraintsValues.
//
// The deployer parameter (when not an empty string) is used as a prefix for
// the name of the SCC and the linked ServiceAccounts.
func NewSecurityContextConstraints(values SecurityContextConstraintsValues) (*secv1.SecurityContextConstraints, error) {
	data, err := NewSecurityContextConstraintsYAML(values)
	if err != nil {
		return nil, err
	}

	scc := &secv1.SecurityContextConstraints{}
	err = yaml.Unmarshal([]byte(data), scc)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", scc, err)
	}

	return scc, nil
}

// internalSecurityContextConstraintsValues extends
// SecurityContextConstraintsValues with some private attributes that may get
// set based on other values.
type internalSecurityContextConstraintsValues struct {
	SecurityContextConstraintsValues

	// Prefix is based on SecurityContextConstraintsValues.Deployer.
	Prefix string
}

// NewSecurityContextConstraintsYAML returns a YAML string where the variables
// in the template have been replaced by the values set in the
// SecurityContextConstraintsValues.
func NewSecurityContextConstraintsYAML(values SecurityContextConstraintsValues) (string, error) {
	var buf bytes.Buffer

	// internalValues is a copy of values, but will get extended with
	// API-internal values
	internalValues := internalSecurityContextConstraintsValues{
		SecurityContextConstraintsValues: values,
	}

	if internalValues.Deployer != "" {
		internalValues.Prefix = internalValues.Deployer + "-"
	}

	tmpl, err := template.New("SCC").Parse(securityContextConstraints)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	err = tmpl.Execute(&buf, internalValues)
	if err != nil {
		return "", fmt.Errorf("failed to replace values in template: %w", err)
	}

	return buf.String(), nil
}
