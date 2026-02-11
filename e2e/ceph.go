/*
Copyright 2026 The Ceph-CSI Authors.

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

package e2e

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	CephMajorSquid    = 19
	CephMajorTentacle = 20
	CephMajorUmbrella = 21
)

// cephVersion is a helper type that converts the standard Ceph version string
// into easily accessible components.
type cephVersion struct {
	major   int
	minor   int
	patch   int
	build   string
	release string
}

func (cv *cephVersion) String() string {
	s := fmt.Sprintf("%d.%d.%d", cv.major, cv.minor, cv.patch)
	if cv.build != "" {
		s = s + "-" + cv.build
	}

	s = fmt.Sprintf("%s (%s)", s, cv.release)

	return s
}

// parse splits the version string into sepate pieces.
// Example: "ceph version 19.2.1-292.el9cp (ba02d58) squid (stable)"
func (cv *cephVersion) UnmarshalJSON(data []byte) error {
	var err error

	parts := strings.Split(strings.Trim(string(data), "\""), " ")
	if len(parts) < 5 {
		return fmt.Errorf("ceph version is expected to be 5 parts or more: %s", string(data))
	}

	// parts[0] and [1] should be "ceph" "version"
	if parts[0] != "ceph" || parts[1] != "version" {
		return fmt.Errorf("ceph version does not start with expected prefix: %s", string(data))
	}

	// parts[2] is the version "19.2.1-292.el9cp"
	versionParts := strings.SplitN(parts[2], ".", 3)
	if len(versionParts) != 3 {
		return fmt.Errorf("ceph version is expected to have 3 numeric parts: %s", string(data))
	}
	cv.major, err = strconv.Atoi(versionParts[0])
	if err != nil {
		return fmt.Errorf("failed to parse major %q into int: %w", versionParts[0], err)
	}
	cv.minor, err = strconv.Atoi(versionParts[1])
	if err != nil {
		return fmt.Errorf("failed to parse minor %q into int: %w", versionParts[1], err)
	}

	patchVersions := strings.SplitN(versionParts[2], "-", 2)
	cv.patch, err = strconv.Atoi(patchVersions[0])
	if err != nil {
		return fmt.Errorf("failed to parse patch %q into int: %w", patchVersions[0], err)
	}
	if len(patchVersions) == 2 {
		cv.build = patchVersions[1]
	}

	// parts[3] is the commit hash, skip it

	// parts[4] is the release name "squid"
	cv.release = parts[4]

	return nil
}

func (cv *cephVersion) GetMajor() int {
	return cv.major
}

func (cv *cephVersion) GetMinor() int {
	return cv.minor
}

func (cv *cephVersion) GetPatch() int {
	return cv.patch
}

func (cv *cephVersion) GetBuild() string {
	return cv.build
}

func (cv *cephVersion) GetRelease() string {
	return cv.release
}

func getCephVersion(f *framework.Framework) (*cephVersion, error) {
	cmd := "ceph --format=json version"
	stdout, stderr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to exec command in toolbox: %w", err)
	}
	if stderr != "" {
		return nil, fmt.Errorf("failed to get Ceph version: %v", stderr)
	}

	var rawVersion struct {
		Version cephVersion `json:"version"`
	}

	err = json.Unmarshal([]byte(stdout), &rawVersion)
	if err != nil {
		return nil, err
	}

	return &rawVersion.Version, nil
}
