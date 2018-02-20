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
	"os/exec"
	"strings"

	"github.com/golang/glog"
)

type decorator interface {
	Open(devicePath, newDevicePrefix string, attributes map[string]string) (bool, string, error)
	Close(target string) error
	GenerateAttributes(attr map[string]string) map[string]string
}

var registeredDecorators map[string]decorator

func registerDecorators(driverName string, dec decorator) {
	if len(registeredDecorators) == 0 {
		registeredDecorators = make(map[string]decorator)
	}
	registeredDecorators[driverName] = dec
	glog.V(3).Infof("register driver %s", driverName)
}

func Open(devicePath, newDevicePrefix string, attr map[string]string) (bool, string, error) {
	var err error
	newDevicePath := ""
	processed := false

	for name, driver := range registeredDecorators {
		glog.V(3).Infof("process device %s through driver %s", devicePath, name)
		processed, newDevicePath, err = driver.Open(devicePath, newDevicePrefix, attr)
		if processed && err == nil {
			devicePath = newDevicePath
		}
	}
	return processed, devicePath, err
}

func GenerateAttributes(attr map[string]string) map[string]string {
	for name, driver := range registeredDecorators {
		glog.V(3).Infof("generate attributes through driver %s", name)
		attr = driver.GenerateAttributes(attr)
	}
	return attr
}

func Close(devicePath string) error {
	cmd := exec.Command("lsblk", "-p", "-P", "-o", "NAME,TYPE", devicePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	str := strings.Replace(string(output), "\"", "", -1)
	lines := strings.Split(str, "\n")
	for i := len(lines); i > 0; i-- {
		l := lines[i-1]
		dev := ""
		devType := ""

		if len(l) <= 0 {
			// Ignore empty line.
			continue
		}
		p := strings.Split(l, " ")
		if len(p) != 2 {
			continue
		}
		for _, v := range p {
			sp := strings.Split(v, "=")
			if sp[0] == "NAME" {
				dev = sp[1]
			}
			if sp[0] == "TYPE" {
				devType = sp[1]
			}
		}
		glog.V(3).Infof("closing device: %s type: %s\n", dev, devType)
		driver, ok := registeredDecorators[devType]
		if ok {
			driver.Close(dev)
		}
	}
	return nil
}
