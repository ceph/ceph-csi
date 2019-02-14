/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"text/template"

	"k8s.io/klog"
)

var cephConfig = []byte(`[global]
auth_cluster_required = cephx
auth_service_required = cephx
auth_client_required = cephx

# Workaround for http://tracker.ceph.com/issues/23446
fuse_set_user_groups = false
`)

const cephKeyring = `[client.{{.UserID}}]
key = {{.Key}}
`

const cephSecret = `{{.Key}}` // #nosec

const (
	cephConfigRoot         = "/etc/ceph"
	cephConfigPath         = "/etc/ceph/ceph.conf"
	cephKeyringFileNameFmt = "ceph.share.%s.client.%s.keyring"
	cephSecretFileNameFmt  = "ceph.share.%s.client.%s.secret" // #nosec
)

var (
	cephKeyringTempl *template.Template
	cephSecretTempl  *template.Template
)

func init() {
	fm := map[string]interface{}{
		"perms": func(readOnly bool) string {
			if readOnly {
				return "r"
			}

			return "rw"
		},
	}

	cephKeyringTempl = template.Must(template.New("keyring").Funcs(fm).Parse(cephKeyring))
	cephSecretTempl = template.Must(template.New("secret").Parse(cephSecret))
}

func createCephConfigRoot() error {
	return os.MkdirAll(cephConfigRoot, 0755) // #nosec
}

func writeCephConfig() error {
	if err := createCephConfigRoot(); err != nil {
		return err
	}

	return ioutil.WriteFile(cephConfigPath, cephConfig, 0640)
}

func writeCephTemplate(fileName string, m os.FileMode, t *template.Template, data interface{}) error {
	if err := createCephConfigRoot(); err != nil {
		return err
	}

	f, err := os.OpenFile(path.Join(cephConfigRoot, fileName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, m)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}

	defer func() {
		if err := f.Close(); err != nil {
			klog.Errorf("failed to close file %s with error %s", f.Name(), err)
		}
	}()

	return t.Execute(f, data)
}

type cephKeyringData struct {
	UserID, Key string
	VolumeID    volumeID
}

func (d *cephKeyringData) writeToFile() error {
	return writeCephTemplate(fmt.Sprintf(cephKeyringFileNameFmt, d.VolumeID, d.UserID), 0600, cephKeyringTempl, d)
}

type cephSecretData struct {
	UserID, Key string
	VolumeID    volumeID
}

func (d *cephSecretData) writeToFile() error {
	return writeCephTemplate(fmt.Sprintf(cephSecretFileNameFmt, d.VolumeID, d.UserID), 0600, cephSecretTempl, d)
}

func getCephSecretPath(volID volumeID, userID string) string {
	return path.Join(cephConfigRoot, fmt.Sprintf(cephSecretFileNameFmt, volID, userID))
}

func getCephKeyringPath(volID volumeID, userID string) string {
	return path.Join(cephConfigRoot, fmt.Sprintf(cephKeyringFileNameFmt, volID, userID))
}
