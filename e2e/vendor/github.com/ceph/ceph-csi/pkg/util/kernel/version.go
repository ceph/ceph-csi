/*
Copyright 2025 The Ceph-CSI Authors.

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

package kernel

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// GetKernelVersion returns the version of the running Unix (like) system from the
// 'utsname' structs 'release' component.
func GetKernelVersion() (string, error) {
	utsname := unix.Utsname{}
	if err := unix.Uname(&utsname); err != nil {
		return "", err
	}

	return strings.TrimRight(string(utsname.Release[:]), "\x00"), nil
}

// KernelVersion holds kernel related information.
type KernelVersion struct {
	Version      int
	PatchLevel   int
	SubLevel     int
	ExtraVersion int    // prefix of the part after the first "-"
	Distribution string // component of full extraversion
	Backport     bool   // backport have a fixed version/patchlevel/sublevel
}

// parseKernelRelease parses a kernel release version string into:
// version, patch version, sub version and extra version.
func parseKernelRelease(release string) (int, int, int, int, error) {
	version := 0
	patchlevel := 0
	minVersions := 2

	extra := ""
	n, err := fmt.Sscanf(release, "%d.%d%s", &version, &patchlevel, &extra)
	if n < minVersions && err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to parse version and patchlevel from %s: %w", release, err)
	}

	sublevel := 0
	extraversion := 0
	if n > minVersions {
		n, err = fmt.Sscanf(extra, ".%d%s", &sublevel, &extra)
		if err != nil && n == 0 && extra != "" && extra[0] != '-' && extra[0] == '.' {
			return 0, 0, 0, 0, fmt.Errorf("failed to parse subversion from %s: %w", release, err)
		}

		extra = strings.TrimPrefix(extra, "-")
		// ignore errors, 1st component of extraversion does not need to be an int
		_, err = fmt.Sscanf(extra, "%d", &extraversion)
		if err != nil {
			// "go lint" wants err to be checked...
			extraversion = 0
		}
	}

	return version, patchlevel, sublevel, extraversion, nil
}

// CheckKernelSupport checks the running kernel and comparing it to known
// versions that have support for required features . Distributors of
// enterprise Linux have backport quota support to previous versions. This
// function checks if the running kernel is one of the versions that have the
// feature/fixes backport.
//
// `uname -r` (or Uname().Utsname.Release has a format like 1.2.3-rc.vendor
// This can be slit up in the following components: - version (1) - patchlevel
// (2) - sublevel (3) - optional, defaults to 0 - extraversion (rc) - optional,
// matching integers only - distribution (.vendor) - optional, match against
// whole `uname -r` string
//
// For matching multiple versions, the kernelSupport type contains a backport
// bool, which will cause matching
// version+patchlevel+sublevel+(>=extraversion)+(~distribution)
//
// In case the backport bool is false, a simple check for higher versions than
// version+patchlevel+sublevel is done.
func CheckKernelSupport(release string, supportedVersions []KernelVersion) bool {
	version, patchlevel, sublevel, extraversion, err := parseKernelRelease(release)
	if err != nil {
		log.ErrorLogMsg("%v", err)

		return false
	}

	// compare running kernel against known versions
	for _, kernel := range supportedVersions {
		if !kernel.Backport {
			// deal with the default case(s), find >= match for version, patchlevel, sublevel
			if version > kernel.Version || (version == kernel.Version && patchlevel > kernel.PatchLevel) ||
				(version == kernel.Version && patchlevel == kernel.PatchLevel && sublevel >= kernel.SubLevel) {
				return true
			}
		} else {
			// specific backport, match distribution initially
			if !strings.Contains(release, kernel.Distribution) {
				continue
			}

			// strict match version, patchlevel, sublevel, and >= match extraversion
			if version == kernel.Version && patchlevel == kernel.PatchLevel &&
				sublevel == kernel.SubLevel && extraversion >= kernel.ExtraVersion {
				return true
			}
		}
	}
	log.WarningLogMsg("kernel %s does not support required features", release)

	return false
}
