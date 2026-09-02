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

package rbd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCgroupQoSParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]string
		want   *cgroupQoS
	}{
		{
			name: "all parameters set",
			params: map[string]string{
				maxReadIops:  "1000",
				maxWriteIops: "2000",
				maxReadBps:   "10485760",
				maxWriteBps:  "20971520",
			},
			want: &cgroupQoS{
				maxReadIops:  "1000",
				maxWriteIops: "2000",
				maxReadBps:   "10485760",
				maxWriteBps:  "20971520",
			},
		},
		{
			name: "partial parameters",
			params: map[string]string{
				maxReadIops:  "1000",
				maxWriteIops: "2000",
			},
			want: &cgroupQoS{
				maxReadIops:  "1000",
				maxWriteIops: "2000",
				maxReadBps:   "max",
				maxWriteBps:  "max",
			},
		},
		{
			name:   "no parameters",
			params: map[string]string{},
			want: &cgroupQoS{
				maxReadIops:  "max",
				maxWriteIops: "max",
				maxReadBps:   "max",
				maxWriteBps:  "max",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseCgroupQoSParams(tt.params)
			if got.maxReadIops != tt.want.maxReadIops {
				t.Errorf("parseCgroupQoSParams() maxReadIops = %v, want %v", got.maxReadIops, tt.want.maxReadIops)
			}
			if got.maxWriteIops != tt.want.maxWriteIops {
				t.Errorf("parseCgroupQoSParams() maxWriteIops = %v, want %v", got.maxWriteIops, tt.want.maxWriteIops)
			}
			if got.maxReadBps != tt.want.maxReadBps {
				t.Errorf("parseCgroupQoSParams() maxReadBps = %v, want %v", got.maxReadBps, tt.want.maxReadBps)
			}
			if got.maxWriteBps != tt.want.maxWriteBps {
				t.Errorf("parseCgroupQoSParams() maxWriteBps = %v, want %v", got.maxWriteBps, tt.want.maxWriteBps)
			}
		})
	}
}

func TestHasCgroupQoSParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]string
		want   bool
	}{
		{
			name: "has maxReadIops",
			params: map[string]string{
				maxReadIops: "1000",
			},
			want: true,
		},
		{
			name: "has maxWriteIops",
			params: map[string]string{
				maxWriteIops: "2000",
			},
			want: true,
		},
		{
			name: "has maxReadBps",
			params: map[string]string{
				maxReadBps: "10485760",
			},
			want: true,
		},
		{
			name: "has maxWriteBps",
			params: map[string]string{
				maxWriteBps: "20971520",
			},
			want: true,
		},
		{
			name:   "no cgroup QoS params",
			params: map[string]string{},
			want:   false,
		},
		{
			name: "empty value",
			params: map[string]string{
				maxReadIops: "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasCgroupQoSParams(tt.params); got != tt.want {
				t.Errorf("hasCgroupQoSParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatIOMax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		qos  *cgroupQoS
		want string
	}{
		{
			name: "all limits set",
			qos: &cgroupQoS{
				deviceID:     "252:0",
				maxReadIops:  "1000",
				maxWriteIops: "2000",
				maxReadBps:   "10485760",
				maxWriteBps:  "20971520",
			},
			want: "252:0 rbps=10485760 wbps=20971520 riops=1000 wiops=2000",
		},
		{
			name: "only IOPS limits",
			qos: &cgroupQoS{
				deviceID:     "252:0",
				maxReadIops:  "1000",
				maxWriteIops: "2000",
				maxReadBps:   "max",
				maxWriteBps:  "max",
			},
			want: "252:0 rbps=max wbps=max riops=1000 wiops=2000",
		},
		{
			name: "only BPS limits",
			qos: &cgroupQoS{
				deviceID:     "252:0",
				maxReadIops:  "max",
				maxWriteIops: "max",
				maxReadBps:   "10485760",
				maxWriteBps:  "20971520",
			},
			want: "252:0 rbps=10485760 wbps=20971520 riops=max wiops=max",
		},
		{
			name: "all max",
			qos: &cgroupQoS{
				deviceID:     "252:0",
				maxReadIops:  "max",
				maxWriteIops: "max",
				maxReadBps:   "max",
				maxWriteBps:  "max",
			},
			want: "252:0 rbps=max wbps=max riops=max wiops=max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.qos.formatIOMax(); got != tt.want {
				t.Errorf("formatIOMax() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateCgroupQoSParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
	}{
		{
			name: "valid parameters",
			params: map[string]string{
				maxReadIops:  "1000",
				maxWriteIops: "2000",
				maxReadBps:   "10485760",
				maxWriteBps:  "20971520",
			},
			wantErr: false,
		},
		{
			name: "invalid maxReadIops",
			params: map[string]string{
				maxReadIops: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid maxWriteIops",
			params: map[string]string{
				maxWriteIops: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid maxReadBps",
			params: map[string]string{
				maxReadBps: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid maxWriteBps",
			params: map[string]string{
				maxWriteBps: "invalid",
			},
			wantErr: true,
		},
		{
			name: "zero maxReadIops",
			params: map[string]string{
				maxReadIops: "0",
			},
			wantErr: false,
		},
		{
			name: "negative maxWriteIops",
			params: map[string]string{
				maxWriteIops: "-1000",
			},
			wantErr: true,
		},
		{
			name: "zero maxReadBps",
			params: map[string]string{
				maxReadBps: "0",
			},
			wantErr: false,
		},
		{
			name: "negative maxWriteBps",
			params: map[string]string{
				maxWriteBps: "-10485760",
			},
			wantErr: true,
		},
		{
			name: "max value resets to unlimited",
			params: map[string]string{
				maxReadIops:  "max",
				maxWriteIops: "max",
				maxReadBps:   "max",
				maxWriteBps:  "max",
			},
			wantErr: false,
		},
		{
			name: "partial max values",
			params: map[string]string{
				maxReadIops: "max",
				maxWriteBps: "1000",
			},
			wantErr: false,
		},
		{
			name:    "empty parameters",
			params:  map[string]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateCgroupQoSParams(tt.params); (err != nil) != tt.wantErr {
				t.Errorf("validateCgroupQoSParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFindPodCgroupPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name    string
		podUID  string
		wantErr bool
	}{
		{
			name:    "empty pod UID returns error",
			podUID:  "",
			wantErr: true,
		},
		{
			name:    "non-existent pod returns error",
			podUID:  "nonexistent-pod-12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := findPodCgroupPath(ctx, tt.podUID)

			if tt.wantErr && err == nil {
				t.Errorf("findPodCgroupPath() expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("findPodCgroupPath() unexpected error: %v", err)
			}
		})
	}
}

// TestPodCgroupCandidates validates the candidate path generation for both
// systemd and cgroupfs cgroup drivers.
func TestPodCgroupCandidates(t *testing.T) {
	t.Parallel()

	podUID := "d9fd4536-e68d-46a1-954b-2f05da70bd7d"
	uidUnderscore := "d9fd4536_e68d_46a1_954b_2f05da70bd7d"

	candidates := podCgroupCandidates(podUID)

	expectedPaths := []string{
		// systemd
		filepath.Join(cgroupV2SystemdBase,
			"kubepods-pod"+uidUnderscore+".slice"),
		filepath.Join(cgroupV2SystemdBase, "kubepods-burstable.slice",
			"kubepods-burstable-pod"+uidUnderscore+".slice"),
		filepath.Join(cgroupV2SystemdBase, "kubepods-besteffort.slice",
			"kubepods-besteffort-pod"+uidUnderscore+".slice"),
		// cgroupfs
		filepath.Join(cgroupV2CgroupfsBase, "pod"+podUID),
		filepath.Join(cgroupV2CgroupfsBase, "burstable", "pod"+podUID),
		filepath.Join(cgroupV2CgroupfsBase, "besteffort", "pod"+podUID),
	}

	if len(candidates) != len(expectedPaths) {
		t.Fatalf("podCgroupCandidates() returned %d candidates, want %d",
			len(candidates), len(expectedPaths))
	}

	for i, got := range candidates {
		if got != expectedPaths[i] {
			t.Errorf("candidate[%d] = %s, want %s", i, got, expectedPaths[i])
		}
	}
}

// TestPodCgroupCandidatesUIDNormalization verifies that systemd paths use
// underscores and cgroupfs paths keep dashes.
func TestPodCgroupCandidatesUIDNormalization(t *testing.T) {
	t.Parallel()

	podUID := "abc-def-123"
	candidates := podCgroupCandidates(podUID)

	for _, c := range candidates[:3] {
		if strings.Contains(c, podUID) {
			t.Errorf("systemd candidate should not contain dashed UID: %s", c)
		}
		if !strings.Contains(c, "abc_def_123") {
			t.Errorf("systemd candidate should contain underscore UID: %s", c)
		}
	}

	for _, c := range candidates[3:] {
		if !strings.Contains(c, podUID) {
			t.Errorf("cgroupfs candidate should contain original dashed UID: %s", c)
		}
	}
}

// TestRegenerateCgroupQoSNoParams verifies that (*rbdVolume).RegenerateCgroupQoS
// is a no-op (returns nil without touching the RBD image) when the
// VolumeAttributesClass carries no cgroup QoS parameters. The image is only
// accessed when there is cgroup QoS to restore, so these paths need no cluster.
func TestRegenerateCgroupQoSNoParams(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name          string
		vacParameters map[string]string
	}{
		{
			name:          "nil parameters",
			vacParameters: nil,
		},
		{
			name:          "empty parameters",
			vacParameters: map[string]string{},
		},
		{
			name: "no cgroup QoS keys",
			vacParameters: map[string]string{
				"unrelated": "value",
			},
		},
		{
			name: "cgroup QoS keys present but empty",
			vacParameters: map[string]string{
				maxReadIops: "",
				maxWriteBps: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// An unconnected volume is safe here because the method must
			// return before touching the cluster.
			rv := &rbdVolume{}
			err := rv.RegenerateCgroupQoS(ctx, tt.vacParameters)
			if err != nil {
				t.Errorf("RegenerateCgroupQoS() error = %v, want nil", err)
			}
		})
	}
}

func TestWriteIOMax(t *testing.T) {
	t.Parallel()

	ioMaxLine := "252:0 rbps=10485760 wbps=20971520 riops=1000 wiops=2000"
	tmpFile := filepath.Join(t.TempDir(), "io.max")

	err := writeIOMax(tmpFile, ioMaxLine)
	if err != nil {
		t.Fatalf("writeIOMax() error = %v", err)
	}

	got, err := os.ReadFile(tmpFile) // #nosec:G304, reading test file.
	if err != nil {
		t.Fatalf("failed to read result file: %v", err)
	}

	want := ioMaxLine + "\n"
	if string(got) != want {
		t.Errorf("writeIOMax() content mismatch: got %q, want %q", string(got), want)
	}
}
