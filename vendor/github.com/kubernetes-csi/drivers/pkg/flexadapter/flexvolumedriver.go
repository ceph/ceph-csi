/*
Copyright 2017 The Kubernetes Authors.

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

package flexadapter

import (
	"sync"
)

type flexVolumeDriver struct {
	sync.Mutex
	driverName          string
	execPath            string
	unsupportedCommands []string
	capabilities        DriverCapabilities
}

// Returns true iff the given command is known to be unsupported.
func (d *flexVolumeDriver) isUnsupported(command string) bool {
	d.Lock()
	defer d.Unlock()
	for _, unsupportedCommand := range d.unsupportedCommands {
		if command == unsupportedCommand {
			return true
		}
	}
	return false
}

func (d *flexVolumeDriver) getExecutable() string {
	return d.execPath
}

// Mark the given commands as unsupported.
func (d *flexVolumeDriver) unsupported(commands ...string) {
	d.Lock()
	defer d.Unlock()
	d.unsupportedCommands = append(d.unsupportedCommands, commands...)
}

func NewFlexVolumeDriver(driverName, driverPath string) (*flexVolumeDriver, error) {

	flexDriver := &flexVolumeDriver{
		driverName: driverName,
		execPath:   driverPath,
	}

	// Initialize the plugin and probe the capabilities
	call := flexDriver.NewDriverCall(initCmd)
	ds, err := call.Run()
	if err != nil {
		return nil, err
	}

	flexDriver.capabilities = *ds.Capabilities

	return flexDriver, nil
}
