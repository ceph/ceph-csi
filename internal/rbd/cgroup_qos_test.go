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
			wantErr: true,
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
			wantErr: true,
		},
		{
			name: "negative maxWriteBps",
			params: map[string]string{
				maxWriteBps: "-10485760",
			},
			wantErr: true,
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

// TestPodCgroupPathConstruction validates the path construction logic
// without requiring actual filesystem access.
func TestPodCgroupPathConstruction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		podUID           string
		qosClass         string
		expectedSlice    string
		expectedPodSlice string
	}{
		{
			name:             "guaranteed pod path",
			podUID:           "abc123-def456",
			qosClass:         "Guaranteed",
			expectedSlice:    kubepodsGuaranteedSlice,
			expectedPodSlice: "kubepods-podabc123_def456.slice",
		},
		{
			name:             "burstable pod path",
			podUID:           "xyz789-uvw123",
			qosClass:         "Burstable",
			expectedSlice:    kubepodsBurstableSlice,
			expectedPodSlice: "kubepods-burstable-podxyz789_uvw123.slice",
		},
		{
			name:             "besteffort pod path",
			podUID:           "pod-best-effort",
			qosClass:         "BestEffort",
			expectedSlice:    kubepodsBestEffortSlice,
			expectedPodSlice: "kubepods-besteffort-podpod_best_effort.slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test UID normalization (hyphens → underscores)
			normalizedUID := strings.ReplaceAll(tt.podUID, "-", "_")

			// Find matching QoS class info
			var qos *struct {
				name      string
				sliceName string
				podPrefix string
			}

			for i := range qosClassInfo {
				if qosClassInfo[i].name == tt.qosClass {
					qos = &qosClassInfo[i]

					break
				}
			}

			if qos == nil {
				t.Fatalf("QoS class %s not found in qosClassInfo", tt.qosClass)
			}

			// Construct expected path using same logic as findPodCgroupPath
			podSliceName := qos.podPrefix + normalizedUID + ".slice"
			expectedPath := filepath.Join(cgroupV2BasePath, tt.expectedSlice, tt.expectedPodSlice)
			constructedPath := filepath.Join(cgroupV2BasePath, qos.sliceName, podSliceName)

			if constructedPath != expectedPath {
				t.Errorf("path construction mismatch:\ngot:  %s\nwant: %s",
					constructedPath, expectedPath)
			}
		})
	}
}

func TestQoSClassOrdering(t *testing.T) {
	t.Parallel()

	// Verify that qosClassInfo array is ordered as expected (Guaranteed first).
	// This ordering is critical for performance - most common QoS class checked first.
	if qosClassInfo[0].name != "Guaranteed" {
		t.Errorf("qosClassInfo[0] should be Guaranteed for optimal performance, got %s", qosClassInfo[0].name)
	}

	if qosClassInfo[1].name != "Burstable" {
		t.Errorf("qosClassInfo[1] should be Burstable, got %s", qosClassInfo[1].name)
	}

	if qosClassInfo[2].name != "BestEffort" {
		t.Errorf("qosClassInfo[2] should be BestEffort, got %s", qosClassInfo[2].name)
	}

	// Verify path prefixes are correctly constructed
	expectedPrefixes := map[string]string{
		"Guaranteed": "kubepods-pod",
		"Burstable":  "kubepods-burstable-pod",
		"BestEffort": "kubepods-besteffort-pod",
	}

	for i := range qosClassInfo {
		expected, ok := expectedPrefixes[qosClassInfo[i].name]
		if !ok {
			t.Errorf("unexpected QoS class name: %s", qosClassInfo[i].name)

			continue
		}

		if qosClassInfo[i].podPrefix != expected {
			t.Errorf("qosClassInfo[%d].podPrefix = %s, want %s",
				i, qosClassInfo[i].podPrefix, expected)
		}
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
