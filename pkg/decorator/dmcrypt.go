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

package decorator

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	driverType    = "crypt" // lsblk type
	MetaDriverKey = "node-attribute-enable-dmcrypt"
	// CSI attributes are map[string]string
	// use string true instead of bool true
	MetaDriverVal = "true"
	MetaAttrKey   = "node-attribute-dmcrypt-passphrase"
)

type dmcrypt struct {
}

var _ decorator = &dmcrypt{}

func init() {
	registerDecorators(driverType, &dmcrypt{})
}

func (d *dmcrypt) Open(devicePath, prefix string, attributes map[string]string) (bool, string, error) {
	if len(devicePath) == 0 {
		return false, devicePath, nil
	}
	passphrase := ""
	dmcryptEnabled := false
	for k, v := range attributes {
		if k == MetaDriverKey && v == MetaDriverVal {
			dmcryptEnabled = true
		}
		if k == MetaAttrKey {
			passphrase = v
		}
	}
	if dmcryptEnabled && len(passphrase) > 0 {
		glog.V(3).Infof("dmcrypt %s", devicePath)
		if ok, err := isLuks(devicePath); err != nil {
			return false, devicePath, err
		} else if !ok {
			// luks format
			if err := luksFormat(devicePath, passphrase); err != nil {
				return false, devicePath, err
			}
		}
		target := prefix + string(uuid.NewUUID())
		if err := luksOpen(devicePath, passphrase, target); err == nil {
			devicePath = "/dev/mapper/" + target
		}
	}
	return true, devicePath, nil
}

func (d *dmcrypt) GenerateAttributes(attr map[string]string) map[string]string {
	dmcryptEnabled := false
	for k, v := range attr {
		if k == MetaDriverKey && v == MetaDriverVal {
			dmcryptEnabled = true
			break
		}
	}
	if dmcryptEnabled {
		attr[MetaAttrKey] = string(uuid.NewUUID())
	}
	return attr
}

func (d *dmcrypt) Close(target string) error {
	glog.V(3).Infof("close %s", target)
	return luksClose(target)
}

func isLuks(devicePath string) (bool, error) {
	glog.V(3).Infof("isLuks %s", devicePath)
	cmd := exec.Command("cryptsetup", "isLuks", devicePath)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	ok := (err.Error() == "exit status 1")
	if ok {
		return false, nil
	}
	return false, err
}

func luksFormat(devicePath, passPhrase string) error {
	glog.V(3).Infof("Luks format %s", devicePath)
	cmd := exec.Command("cryptsetup", "luksFormat", devicePath, "-")
	cmd.Stdin = bytes.NewReader([]byte(passPhrase))
	err := cmd.Run()
	if err != nil {
		glog.Warningf("failed to luks format %s: %v", devicePath, err)
	}
	return err
}

func luksOpen(devicePath, passPhrase, target string) error {
	glog.V(3).Infof("Luks open %s", devicePath)
	cmd := exec.Command("cryptsetup", "luksOpen", devicePath, target)
	cmd.Stdin = bytes.NewReader([]byte(passPhrase))
	err := cmd.Run()
	if err != nil {
		glog.Warningf("failed to luks open %s as %s: %v", devicePath, target, err)
	}
	return err
}

func luksClose(devicePath string) error {
	strs := strings.Split(devicePath, "/")
	target := strs[len(strs)-1]
	cmd := exec.Command("cryptsetup", "luksClose", target)
	err := cmd.Run()
	if err != nil {
		glog.Warningf("failed to luks close %s: %v", target, err)
	}
	return err
}
