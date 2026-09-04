/*
 * cgroup.go - Read CPU and memory limits from Linux cgroups v2.
 *
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

// Package cgroup reads CPU and memory resource limits from Linux control
// groups (cgroup v2).
//
// References:
//   - cgroups(7):           https://man7.org/linux/man-pages/man7/cgroups.7.html
//   - cgroup v2 (cpu.max, memory.max): https://docs.kernel.org/admin-guide/cgroup-v2.html
//   - /proc/self/cgroup:    https://man7.org/linux/man-pages/man7/cgroups.7.html (see "/proc files")
package cgroup

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Errors.
var (
	// ErrNoLimit indicates that no cgroup limit is set.
	ErrNoLimit = errors.New("no cgroup limit set")

	// ErrV1Detected indicates that cgroup v1 controllers were found. Only v2 is
	// supported.
	ErrV1Detected = errors.New("cgroup v1 detected; only v2 is supported")
)

// Cgroup provides access to cgroup v2 resource limits. Create one with
// New or NewFromRoot.
type Cgroup struct {
	// cgroupDir is the resolved filesystem path to the cgroup directory
	// (e.g. /sys/fs/cgroup/user.slice/...).
	cgroupDir string
}

// New returns a Cgroup by reading /proc/self/cgroup on the live system.
func New() (Cgroup, error) {
	return NewFromRoot("/")
}

// NewFromRoot is like New but resolves all filesystem paths relative to
// root instead of "/". This is useful for testing with a mock filesystem.
func NewFromRoot(root string) (Cgroup, error) {
	groupPath, err := parseProcCgroup(filepath.Join(root, "proc/self/cgroup"))
	if err != nil {
		return Cgroup{}, err
	}
	return Cgroup{
		cgroupDir: filepath.Join(root, "sys/fs/cgroup", groupPath),
	}, nil
}

// CPUQuota returns the CPU quota as a fractional number of CPUs (e.g. 0.5
// means half a core). Returns ErrNoLimit if no CPU limit is configured.
func (c Cgroup) CPUQuota() (float64, error) {
	data, err := c.readFile("cpu.max")
	if err != nil {
		return 0, err
	}
	return parseCPUMax(data)
}

// MemoryLimit returns the cgroup memory limit in bytes. Returns ErrNoLimit
// if no memory limit is configured.
func (c Cgroup) MemoryLimit() (int64, error) {
	data, err := c.readFile("memory.max")
	if err != nil {
		return 0, err
	}
	return parseMemoryMax(data)
}

func (c Cgroup) readFile(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(c.cgroupDir, path))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoLimit
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// parseProcCgroup parses /proc/self/cgroup and returns the cgroup v2 group
// path. The v2 entry is the line with hierarchy-ID "0" and an empty
// controller list: "0::<path>".
//
// Returns an error if v1 controllers are detected or no v2 entry is found.
//
// https://man7.org/linux/man-pages/man7/cgroups.7.html
func parseProcCgroup(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var v2Path string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] == "0" && parts[1] == "" {
			v2Path = parts[2]
		} else if parts[1] != "" {
			return "", ErrV1Detected
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if v2Path == "" {
		return "", fmt.Errorf("no cgroup v2 entry found in %s", path)
	}
	return v2Path, nil
}

func parseCPUMax(content string) (float64, error) {
	fields := strings.Fields(content)
	if len(fields) == 0 || len(fields) > 2 {
		return 0, fmt.Errorf("unexpected cpu.max format: %q", content)
	}
	if fields[0] == "max" {
		return 0, ErrNoLimit
	}
	quota, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing cpu.max quota: %w", err)
	}
	period := 100000.0
	if len(fields) == 2 {
		period, err = strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return 0, fmt.Errorf("parsing cpu.max period: %w", err)
		}
		if period == 0 {
			return 0, fmt.Errorf("cpu.max period is zero")
		}
	}
	return quota / period, nil
}

func parseMemoryMax(content string) (int64, error) {
	if content == "max" {
		return 0, ErrNoLimit
	}
	v, err := strconv.ParseInt(content, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing memory.max: %w", err)
	}
	return v, nil
}
