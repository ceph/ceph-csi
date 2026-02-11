/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"
)

func TestCephVersionUnmarshalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		// name for this test
		name string

		// input version
		version string

		// resulting version components
		major   int
		minor   int
		patch   int
		build   string
		release string

		// expects error, or success
		shouldFail bool
	}{
		{
			name:       "valid Ceph Squid version",
			version:    "ceph version 19.2.1-292.el9cp (ba02d58) squid (stable)",
			major:      19,
			minor:      2,
			patch:      1,
			build:      "292.el9cp",
			release:    "squid",
			shouldFail: false,
		},
		{
			name:       "invalid version prefix",
			version:    "Fedora Linux 43 (Forty Three)",
			shouldFail: true,
		},
		{
			name:       "too few version numbers",
			version:    "ceph version 19 (ba02d58) squid (stable)",
			shouldFail: true,
		},
		{
			name:       "invalid version numbers",
			version:    "ceph version 0x13.2.1-292.el9cp (ba02d58) squid (stable)",
			shouldFail: true,
		},
		{
			name:       "missing build id",
			version:    "ceph version 19.2.1 (ba02d58) squid (stable)",
			major:      19,
			minor:      2,
			patch:      1,
			build:      "",
			release:    "squid",
			shouldFail: false,
		},
		{
			name:       "missing release name",
			version:    "ceph version 19.2.1-292.el9cp (ba02d58)",
			shouldFail: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cv := &cephVersion{}
			err := cv.UnmarshalJSON([]byte(tt.version))
			switch {
			case err != nil && !tt.shouldFail:
				t.Errorf("failed unmarshalling version: %v", err)
			case err == nil && tt.shouldFail:
				t.Errorf("failure expected, but did not get an error")
			case err != nil && tt.shouldFail:
				return
			}

			if cv.GetMajor() != tt.major {
				t.Errorf("expecred major %d, got %d", tt.major, cv.GetMajor())
			}
			if cv.GetMinor() != tt.minor {
				t.Errorf("expecred major %d, got %d", tt.minor, cv.GetMinor())
			}
			if cv.GetPatch() != tt.patch {
				t.Errorf("expecred patch %d, got %d", tt.patch, cv.GetPatch())
			}
			if cv.GetBuild() != tt.build {
				t.Errorf("expecred build %q, got %q", tt.build, cv.GetBuild())
			}
			if cv.GetRelease() != tt.release {
				t.Errorf("expecred release %q, got %q", tt.release, cv.GetRelease())
			}
		})
	}
}
