/*
Copyright 2018 The Ceph-CSI Authors.

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
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// ConstructMountOptions returns only unique mount options in slice
func ConstructMountOptions(mountOptions []string, volCap *csi.VolumeCapability) []string {
	if m := volCap.GetMount(); m != nil {
		hasOption := func(options []string, opt string) bool {
			for _, o := range options {
				if o == opt {
					return true
				}
			}
			return false
		}
		for _, f := range m.MountFlags {
			if !hasOption(mountOptions, f) {
				mountOptions = append(mountOptions, f)
			}
		}
	}
	return mountOptions
}

// Contains checks the opt is present in mountOptions
func Contains(mountOptions []string, opt string) bool {
	for _, mnt := range mountOptions {
		if mnt == opt {
			return true
		}
	}
	return false
}

// CheckROMountFlag checks the options has the `ro flag`
func CheckROMountFlag(options map[string]string, param, flag string) bool {
	if o, ok := options[param]; ok {
		mOpt := strings.Split(o, ",")
		for _, m := range mOpt {
			if m == flag {
				return true
			}
		}
	}
	return false
}
