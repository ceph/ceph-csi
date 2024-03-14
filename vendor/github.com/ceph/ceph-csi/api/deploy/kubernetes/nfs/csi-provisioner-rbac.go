/*
Copyright 2024 The Ceph-CSI Authors.

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

package nfs

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"strings"

	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/ceph/ceph-csi/api/deploy/kubernetes"
)

//go:embed csi-provisioner-rbac-sa.yaml
var csiProvisionerServiceAccount string

//go:embed csi-provisioner-rbac-cr.yaml
var csiProvisionerClusterRole string

//go:embed csi-provisioner-rbac-crb.yaml
var csiProvisionerClusterRoleBinding string

//go:embed csi-provisioner-rbac-r.yaml
var csiProvisionerRole string

//go:embed csi-provisioner-rbac-rb.yaml
var csiProvisionerRoleBinding string

var CSIProvisionerRBACDefaults = kubernetes.CSIProvisionerRBACValues{
	Namespace:      "default",
	ServiceAccount: "nfs-csi-provisioner",
}

// NewCSIProvisionerRBAC takes a driver name from the CSIProvisionerRBACValues
// struct and replaces the value in the template. A CSIProvisionerRBAC object
// is returned which can be used to create permissions for the provisioner in
// the Kubernetes cluster.
func NewCSIProvisionerRBAC(values kubernetes.CSIProvisionerRBACValues) (kubernetes.CSIProvisionerRBAC, error) {
	sa := &corev1.ServiceAccount{}
	sa.Namespace = values.Namespace
	sa.Name = values.ServiceAccount

	cr, err := newClusterRole(values)
	if err != nil {
		return nil, err
	}

	crb, err := newClusterRoleBinding(values)
	if err != nil {
		return nil, err
	}

	r, err := newRole(values)
	if err != nil {
		return nil, err
	}

	rb, err := newRoleBinding(values)
	if err != nil {
		return nil, err
	}

	return &csiProvisionerRBAC{
		serviceAccount:     sa,
		clusterRole:        cr,
		clusterRoleBinding: crb,
		role:               r,
		roleBinding:        rb,
	}, nil
}

func NewCSIProvisionerRBACYAML(values kubernetes.CSIProvisionerRBACValues) (string, error) {
	docs := []string{}

	data, err := newYAML("csiProvisionerServiceAccount", csiProvisionerServiceAccount, values)
	if err != nil {
		return "", err
	}
	docs = append(docs, data)

	data, err = newYAML("csiProvisionerClusterRole", csiProvisionerClusterRole, values)
	if err != nil {
		return "", err
	}
	docs = append(docs, data)

	data, err = newYAML("csiProvisionerClusterRoleBinding", csiProvisionerClusterRoleBinding, values)
	if err != nil {
		return "", err
	}
	docs = append(docs, data)

	data, err = newYAML("csiProvisionerRole", csiProvisionerRole, values)
	if err != nil {
		return "", err
	}
	docs = append(docs, data)

	data, err = newYAML("csiProvisionerRoleBinding", csiProvisionerRoleBinding, values)
	if err != nil {
		return "", err
	}
	docs = append(docs, data)

	return strings.Join(docs, "\n"), nil
}


func newYAML(name, data string, values kubernetes.CSIProvisionerRBACValues) (string, error) {
	var buf bytes.Buffer

	tmpl, err := template.New(name).Parse(data)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	err = tmpl.Execute(&buf, values)
	if err != nil {
		return "", fmt.Errorf("failed to replace values in template: %w", err)
	}

	return buf.String(), nil
}

func newServiceAccount(values kubernetes.CSIProvisionerRBACValues) (*corev1.ServiceAccount, error) {
	data, err := newYAML("csiProvisionerServiceAccount", csiProvisionerServiceAccount, values)
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{}
	err = yaml.Unmarshal([]byte(data), sa)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", sa, err)
	}

	return sa, nil
}

func newClusterRole(values kubernetes.CSIProvisionerRBACValues) (*rbacv1.ClusterRole, error) {
	data, err := newYAML("csiProvisionerClusterRole", csiProvisionerClusterRole, values)
	if err != nil {
		return nil, err
	}

	cr := &rbacv1.ClusterRole{}
	err = yaml.Unmarshal([]byte(data), cr)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", cr, err)
	}

	return cr, nil
}

func newClusterRoleBinding(values kubernetes.CSIProvisionerRBACValues) (*rbacv1.ClusterRoleBinding, error) {
	data, err := newYAML("csiProvisionerClusterRoleBinding", csiProvisionerClusterRoleBinding, values)
	if err != nil {
		return nil, err
	}

	crb := &rbacv1.ClusterRoleBinding{}
	err = yaml.Unmarshal([]byte(data), crb)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", crb, err)
	}

	return crb, nil
}

func newRole(values kubernetes.CSIProvisionerRBACValues) (*rbacv1.Role, error) {
	data, err := newYAML("csiProvisionerRole", csiProvisionerRole, values)
	if err != nil {
		return nil, err
	}

	r := &rbacv1.Role{}
	err = yaml.Unmarshal([]byte(data), r)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", r, err)
	}

	return r, nil
}

func newRoleBinding(values kubernetes.CSIProvisionerRBACValues) (*rbacv1.RoleBinding, error) {
	data, err := newYAML("csiProvisionerRoleBinding", csiProvisionerRoleBinding, values)
	if err != nil {
		return nil, err
	}

	rb := &rbacv1.RoleBinding{}
	err = yaml.Unmarshal([]byte(data), rb)
	if err != nil {
		return nil, fmt.Errorf("failed convert YAML to %T: %w", rb, err)
	}

	return rb, nil
}

type csiProvisionerRBAC struct {
	serviceAccount     *corev1.ServiceAccount
	clusterRole        *rbacv1.ClusterRole
	clusterRoleBinding *rbacv1.ClusterRoleBinding
	role               *rbacv1.Role
	roleBinding        *rbacv1.RoleBinding
}

func (rbac *csiProvisionerRBAC) GetServiceAccount() *corev1.ServiceAccount {
	return rbac.serviceAccount
}

func (rbac *csiProvisionerRBAC) GetClusterRole() *rbacv1.ClusterRole {
	return rbac.clusterRole
}

func (rbac *csiProvisionerRBAC) GetClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return rbac.clusterRoleBinding
}

func (rbac *csiProvisionerRBAC) GetRole() *rbacv1.Role {
	return rbac.role
}

func (rbac *csiProvisionerRBAC) GetRoleBinding() *rbacv1.RoleBinding {
	return rbac.roleBinding
}
