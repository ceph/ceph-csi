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
	ctx := t.Context()

	tests := map[string]string{
		baseIops:      "3000",
		baseReadIops:  "2000",
		baseWriteIops: "1000",
	}
	wants := map[string]string{
		iopsLimit:      "3000",
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
		baseIops:      "3000",
		baseReadIops:  "2000",
		baseWriteIops: "1000",
		baseBps:       "314572800",
		baseReadBps:   "209715200",
		baseWriteBps:  "104857600",
	}
	wants = map[string]string{
		iopsLimit:      "3000",
		readIopsLimit:  "2000",
		writeIopsLimit: "1000",
		bpsLimit:       "314572800",
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
		baseIops:         "3000",
		baseReadIops:     "2000",
		baseWriteIops:    "1000",
		baseBps:          "314572800",
		baseReadBps:      "209715200",
		baseWriteBps:     "104857600",
		iopsPerGiB:       "30",
		readIopsPerGiB:   "20",
		writeIopsPerGiB:  "10",
		bpsPerGiB:        "3145728",
		readBpsPerGiB:    "2097152",
		writeBpsPerGiB:   "1048576",
		baseVolSizeBytes: "21474836480",
	}
	wants = map[string]string{
		iopsLimit:      "3000",
		readIopsLimit:  "2000",
		writeIopsLimit: "1000",
		bpsLimit:       "314572800",
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
		iopsLimit:      "5400",
		readIopsLimit:  "3600",
		writeIopsLimit: "1800",
		bpsLimit:       "566231040",
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

	tests = map[string]string{
		baseIops:         "3000",
		maxIops:          "15000",
		baseReadIops:     "2000",
		maxReadIops:      "10000",
		baseWriteIops:    "1000",
		maxWriteIops:     "5000",
		baseBps:          "314572800",
		maxBps:           "1572864000",
		baseReadBps:      "209715200",
		maxReadBps:       "1048576000",
		baseWriteBps:     "104857600",
		maxWriteBps:      "524288000",
		iopsPerGiB:       "30",
		readIopsPerGiB:   "20",
		writeIopsPerGiB:  "10",
		bpsPerGiB:        "3145728",
		readBpsPerGiB:    "2097152",
		writeBpsPerGiB:   "1048576",
		baseVolSizeBytes: "21474836480",
	}
	wants = map[string]string{
		iopsLimit:      "8400",
		readIopsLimit:  "5600",
		writeIopsLimit: "2800",
		bpsLimit:       "880803840",
		readBpsLimit:   "587202560",
		writeBpsLimit:  "293601280",
	}
	rv = rbdVolume{}
	rv.RequestedVolSize = int64(oneGB) * 200
	err = rv.SetQOS(ctx, tests)
	if err != nil {
		t.Errorf("SetQOS failed: %v", err)
	}
	checkQOS(t, rv.QosParameters, wants)

	wants = map[string]string{
		iopsLimit:      "15000",
		readIopsLimit:  "10000",
		writeIopsLimit: "5000",
		bpsLimit:       "1572864000",
		readBpsLimit:   "1048576000",
		writeBpsLimit:  "524288000",
	}
	rv = rbdVolume{}
	rv.RequestedVolSize = int64(oneGB) * 600
	err = rv.SetQOS(ctx, tests)
	if err != nil {
		t.Errorf("SetQOS failed: %v", err)
	}
	checkQOS(t, rv.QosParameters, wants)
}
