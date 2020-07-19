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
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// LuksFormat sets up volume as an encrypted LUKS partition.
func LuksFormat(devicePath, passphrase string) (stdout, stderr []byte, err error) {
	return execCryptsetupCommand(&passphrase, "-q", "luksFormat", "--hash", "sha256", devicePath, "-d", "/dev/stdin")
}

// LuksOpen opens LUKS encrypted partition and sets up a mapping.
func LuksOpen(devicePath, mapperFile, passphrase string) (stdout, stderr []byte, err error) {
	return execCryptsetupCommand(&passphrase, "luksOpen", devicePath, mapperFile, "-d", "/dev/stdin")
}

// LuksClose removes existing mapping.
func LuksClose(mapperFile string) (stdout, stderr []byte, err error) {
	return execCryptsetupCommand(nil, "luksClose", mapperFile)
}

// LuksStatus returns encryption status of a provided device.
func LuksStatus(mapperFile string) (stdout, stderr []byte, err error) {
	return execCryptsetupCommand(nil, "status", mapperFile)
}

func execCryptsetupCommand(stdin *string, args ...string) (stdout, stderr []byte, err error) {
	var (
		program       = "cryptsetup"
		cmd           = exec.Command(program, args...) // nolint: gosec, #nosec
		sanitizedArgs = StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}

	if err := cmd.Run(); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("an error (%v)"+
			" occurred while running %s args: %v", err, program, sanitizedArgs)
	}

	return stdoutBuf.Bytes(), nil, nil
}
