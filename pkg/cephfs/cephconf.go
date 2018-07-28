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
	"os"
	"path"
	"text/template"
)

const cephConfig = `[global]
mon_host = {{.Monitors}}
auth_cluster_required = cephx
auth_service_required = cephx
auth_client_required = cephx

# Workaround for http://tracker.ceph.com/issues/23446
fuse_set_user_groups = false
`

const cephKeyring = `[client.{{.UserId}}]
key = {{.Key}}
`

const cephSecret = `{{.Key}}`

const (
	cephConfigRoot         = "/etc/ceph"
	cephConfigFileNameFmt  = "ceph.share.%s.conf"
	cephKeyringFileNameFmt = "ceph.share.%s.client.%s.keyring"
	cephSecretFileNameFmt  = "ceph.share.%s.client.%s.secret"
)

var (
	cephConfigTempl  *template.Template
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

	cephConfigTempl = template.Must(template.New("config").Parse(cephConfig))
	cephKeyringTempl = template.Must(template.New("keyring").Funcs(fm).Parse(cephKeyring))
	cephSecretTempl = template.Must(template.New("secret").Parse(cephSecret))
}

type cephConfigWriter interface {
	writeToFile() error
}

type cephConfigData struct {
	Monitors string
	VolumeID volumeID
}

func writeCephTemplate(fileName string, m os.FileMode, t *template.Template, data interface{}) error {
	if err := os.MkdirAll(cephConfigRoot, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path.Join(cephConfigRoot, fileName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, m)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}

	defer f.Close()

	return t.Execute(f, data)
}

func (d *cephConfigData) writeToFile() error {
	return writeCephTemplate(fmt.Sprintf(cephConfigFileNameFmt, d.VolumeID), 0640, cephConfigTempl, d)
}

type cephKeyringData struct {
	UserId, Key string
	VolumeID    volumeID
}

func (d *cephKeyringData) writeToFile() error {
	return writeCephTemplate(fmt.Sprintf(cephKeyringFileNameFmt, d.VolumeID, d.UserId), 0600, cephKeyringTempl, d)
}

type cephSecretData struct {
	UserId, Key string
	VolumeID    volumeID
}

func (d *cephSecretData) writeToFile() error {
	return writeCephTemplate(fmt.Sprintf(cephSecretFileNameFmt, d.VolumeID, d.UserId), 0600, cephSecretTempl, d)
}

func getCephSecretPath(volId volumeID, userId string) string {
	return path.Join(cephConfigRoot, fmt.Sprintf(cephSecretFileNameFmt, volId, userId))
}

func getCephKeyringPath(volId volumeID, userId string) string {
	return path.Join(cephConfigRoot, fmt.Sprintf(cephKeyringFileNameFmt, volId, userId))
}

func getCephConfPath(volId volumeID) string {
	return path.Join(cephConfigRoot, fmt.Sprintf(cephConfigFileNameFmt, volId))
}
