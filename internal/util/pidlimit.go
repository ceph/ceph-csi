/*
Copyright 2019 The Ceph-CSI Authors.

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
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	procCgroup            = "/proc/self/cgroup"
	sysPidsMaxFmtCgroupV1 = "/sys/fs/cgroup/pids%s/pids.max"
	sysPidsMaxFmtCgroupV2 = "/sys/fs/cgroup%s/pids.max"
)

// getCgroupPidsFile return the cgroups "pids.max" file of the
// current process
// For cgroup v1, find the line containing the pids group from the /proc/self/cgroup file
// $ grep ':pids:' /proc/self/cgroup
// 7:pids:/kubepods.slice/kubepods-besteffort.slice/....scope
// $ cat /sys/fs/cgroup/pids + *.scope + /pids.max.
// The entry for cgroup v2 is always in the format "0::...scope", no subsystem given.
// (see https://www.kernel.org/doc/Documentation/cgroup-v2.txt)
func getCgroupPidsFile() (string, error) {
	cgroup, err := os.Open(procCgroup)
	if err != nil {
		return "", err
	}
	defer cgroup.Close() // #nosec: error on close is not critical here

	pidsMax := ""
	scanner := bufio.NewScanner(cgroup)
	var slice string
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 3)
		if parts == nil || len(parts) < 3 {
			continue
		}
		// No cgroup subsystem given, then it is cgroupv2
		if parts[0] == "0" && parts[1] == "" {
			slice = parts[2]
			pidsMax = fmt.Sprintf(sysPidsMaxFmtCgroupV2, slice)

			break
		}
		if parts[1] == "pids" {
			slice = parts[2]
			pidsMax = fmt.Sprintf(sysPidsMaxFmtCgroupV1, slice)

			break
		}
	}
	if slice == "" {
		return "", fmt.Errorf("could not find a cgroup for 'pids'")
	}

	return pidsMax, nil
}

// GetPIDLimit returns the current PID limit, or an error. A value of -1
// translates to "max".
func GetPIDLimit() (int, error) {
	pidsMax, err := getCgroupPidsFile()
	if err != nil {
		return 0, err
	}

	f, err := os.Open(pidsMax) // #nosec - intended reading from /sys/...
	if err != nil {
		return 0, err
	}
	defer f.Close() // #nosec: error on close is not critical here

	maxPidsStr, err := bufio.NewReader(f).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	maxPidsStr = strings.TrimRight(maxPidsStr, "\n")

	maxPids := -1
	if maxPidsStr != "max" {
		maxPids, err = strconv.Atoi(maxPidsStr)
		if err != nil {
			return 0, err
		}
	}

	return maxPids, nil
}

// SetPIDLimit configures the given PID limit for the current process. A value
// of -1 translates to "max".
func SetPIDLimit(limit int) error {
	limitStr := "max"
	if limit != -1 {
		limitStr = fmt.Sprintf("%d", limit)
	}

	pidsMax, err := getCgroupPidsFile()
	if err != nil {
		return err
	}

	f, err := os.Create(pidsMax)
	if err != nil {
		return err
	}

	_, err = f.WriteString(limitStr)
	if err != nil {
		f.Close() // #nosec: a write error will be more useful to return

		return err
	}

	return f.Close()
}
