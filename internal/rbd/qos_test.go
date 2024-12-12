/*
Copyright 2024 The Ceph-CSI Authors.

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
	"testing"
)

func checkQOS(
	t *testing.T,
	target map[string]string,
	wants map[string]string,
) {
	t.Helper()

	for k, v := range wants {
		if r, ok := target[k]; ok {
			if v != r {
				t.Errorf("SetQOS: %s: %s, want %s", k, target[k], v)
			}
		} else {
			t.Errorf("SetQOS: missing qos %s", k)
		}
	}
}

func TestSetQOS(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()

	tests := map[string]string{
		baseReadIops:  "2000",
		baseWriteIops: "1000",
	}
	wants := map[string]string{
		readIopsLimit:  "2000",
		writeIopsLimit: "1000",
	}
	rv := rbdVolume{}
	rv.RequestedVolSize = int64(oneGB)
	err := rv.SetQOS(ctx, tests)
	if err != nil {
		t.Errorf("SetQOS failed: %v", err)
	}
	checkQOS(t, rv.QosParameters, wants)

	tests = map[string]string{
		baseReadIops:            "2000",
		baseWriteIops:           "1000",
		baseReadBytesPerSecond:  "209715200",
		baseWriteBytesPerSecond: "104857600",
	}
	wants = map[string]string{
		readIopsLimit:  "2000",
		writeIopsLimit: "1000",
		readBpsLimit:   "209715200",
		writeBpsLimit:  "104857600",
	}
	rv = rbdVolume{}
	rv.RequestedVolSize = int64(oneGB)
	err = rv.SetQOS(ctx, tests)
	if err != nil {
		t.Errorf("SetQOS failed: %v", err)
	}
	checkQOS(t, rv.QosParameters, wants)

	tests = map[string]string{
		baseReadIops:            "2000",
		baseWriteIops:           "1000",
		baseReadBytesPerSecond:  "209715200",
		baseWriteBytesPerSecond: "104857600",
		readIopsPerGiB:          "20",
		writeIopsPerGiB:         "10",
		readBpsPerGiB:           "2097152",
		writeBpsPerGiB:          "1048576",
		baseVolSizeBytes:        "21474836480",
	}
	wants = map[string]string{
		readIopsLimit:  "2000",
		writeIopsLimit: "1000",
		readBpsLimit:   "209715200",
		writeBpsLimit:  "104857600",
	}
	rv = rbdVolume{}
	rv.RequestedVolSize = int64(oneGB) * 20
	err = rv.SetQOS(ctx, tests)
	if err != nil {
		t.Errorf("SetQOS failed: %v", err)
	}
	checkQOS(t, rv.QosParameters, wants)

	wants = map[string]string{
		readIopsLimit:  "3600",
		writeIopsLimit: "1800",
		readBpsLimit:   "377487360",
		writeBpsLimit:  "188743680",
	}
	rv = rbdVolume{}
	rv.RequestedVolSize = int64(oneGB) * 100
	err = rv.SetQOS(ctx, tests)
	if err != nil {
		t.Errorf("SetQOS failed: %v", err)
	}
	checkQOS(t, rv.QosParameters, wants)
}
