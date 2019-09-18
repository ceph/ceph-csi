/*
Copyright 2014 The Kubernetes Authors.

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
	"fmt"

	"github.com/pkg/errors"

	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
	utilexec "k8s.io/utils/exec"
)

const (
	// 'fsck' found errors and corrected them
	fsckErrorsCorrected = 1
	// 'fsck' found errors but exited without correcting them
	fsckErrorsUncorrected = 4
)

// rFormatAndMount probes a device to see if it is formatted.
// Namely it checks to see if a file system is present. If so it
// mounts it otherwise the device is formatted first then mounted.
type RFormatAndMount struct {
	mount.SafeFormatAndMount
}

// formatAndMount uses unix utils to format and mount the given disk
// nolint: gocyclo
func (mounter *RFormatAndMount) FormatAndMount(source, target, fstype string, options []string) error {
	readOnly := false
	for _, option := range options {
		if option == "ro" {
			readOnly = true
			break
		}
	}

	options = append(options, "defaults")

	if !readOnly {
		// Run fsck on the disk to fix repairable issues, only do this for volumes requested as rw.
		klog.V(4).Infof("Checking for issues with fsck on disk: %s", source)
		args := []string{"-a", source}
		out, err := mounter.Exec.Run("fsck", args...)
		if err != nil {
			ee, isExitError := err.(utilexec.ExitError)
			switch {
			case err == utilexec.ErrExecutableNotFound:
				klog.Warningf("'fsck' not found on system; continuing mount without running 'fsck'")
			case isExitError && ee.ExitStatus() == fsckErrorsCorrected:
				klog.Infof("Device %s has errors which were corrected by fsck", source)
			case isExitError && ee.ExitStatus() == fsckErrorsUncorrected:
				return fmt.Errorf("'fsck' found errors on device %s but could not correct them: %s", source, string(out))
			case isExitError && ee.ExitStatus() > fsckErrorsUncorrected:
				klog.Infof("`fsck` error %s", string(out))
			}
		}
	}

	// Try to mount the disk
	klog.V(4).Infof("Attempting to mount disk: %s %s %s", fstype, source, target)
	mountErr := mounter.Interface.Mount(source, target, fstype, options)
	if mountErr != nil {
		// Mount failed. This indicates either that the disk is unformatted or
		// it contains an unexpected filesystem.
		existingFormat, err := mounter.GetDiskFormat(source)
		if err != nil {
			return err
		}
		if existingFormat == "" {
			if readOnly {
				// Don't attempt to format if mounting as readonly, return an error to reflect this.
				return errors.New("failed to mount unformatted volume as read only")
			}

			// Disk is unformatted so format it.
			args := []string{source}
			// Use 'ext4' as the default
			if fstype == "" {
				fstype = "ext4"
			}

			if fstype == "ext4" || fstype == "ext3" {
				args = []string{
					"-F",           // Force flag
					"-m0",          // Zero blocks reserved for super-user
					"-E nodiscard", // Do not attempt to discard blocks at mkfs time - Mainly used for RBD performance.
					source,
				}

			} else if fstype == "xfs" {
				args = []string{
					"-K", // Do not attempt to discard blocks at mkfs time - Mainly used for RBD performance.
					source,
				}
			}
			klog.Infof("Disk %q appears to be unformatted, attempting to format as type: %q with options: %v", source, fstype, args)
			_, err := mounter.Exec.Run("mkfs."+fstype, args...)
			if err == nil {
				// the disk has been formatted successfully try to mount it again.
				klog.Infof("Disk successfully formatted (mkfs): %s - %s %s", fstype, source, target)
				return mounter.Interface.Mount(source, target, fstype, options)
			}
			klog.Errorf("format of disk %q failed: type:(%q) target:(%q) options:(%q)error:(%v)", source, fstype, target, options, err)
			return err
		}
		// Disk is already formatted and failed to mount
		if fstype == "" || fstype == existingFormat {
			// This is mount error
			return mountErr
		}
		// Block device is formatted with unexpected filesystem, let the user know
		return fmt.Errorf("failed to mount the volume as %q, it already contains %s. Mount error: %v", fstype, existingFormat, mountErr)

	}
	return mountErr
}
