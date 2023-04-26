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
	"strconv"
	"strings"
)

// Limit memory used by Argon2i PBKDF to 32 MiB.
const cryptsetupPBKDFMemoryLimit = 32 << 10 // 32768 KiB

// LuksFormat sets up volume as an encrypted LUKS partition.
func LuksFormat(devicePath, passphrase string) (string, string, error) {
	return execCryptsetupCommand(
		&passphrase,
		"-q",
		"luksFormat",
		"--type",
		"luks2",
		"--hash",
		"sha256",
		"--pbkdf-memory",
		strconv.Itoa(cryptsetupPBKDFMemoryLimit),
		devicePath,
		"-d",
		"/dev/stdin")
}

// LuksOpen opens LUKS encrypted partition and sets up a mapping.
func LuksOpen(devicePath, mapperFile, passphrase string) (string, string, error) {
	// cryptsetup option --disable-keyring (introduced with cryptsetup v2.0.0)
	// will be ignored with luks1
	return execCryptsetupCommand(&passphrase, "luksOpen", devicePath, mapperFile, "--disable-keyring", "-d", "/dev/stdin")
}

// LuksResize resizes LUKS encrypted partition.
func LuksResize(mapperFile string) (string, string, error) {
	return execCryptsetupCommand(nil, "resize", mapperFile)
}

// LuksClose removes existing mapping.
func LuksClose(mapperFile string) (string, string, error) {
	return execCryptsetupCommand(nil, "luksClose", mapperFile)
}

// LuksStatus returns encryption status of a provided device.
func LuksStatus(mapperFile string) (string, string, error) {
	return execCryptsetupCommand(nil, "status", mapperFile)
}

func execCryptsetupCommand(stdin *string, args ...string) (string, string, error) {
	var (
		program       = "cryptsetup"
		cmd           = exec.Command(program, args...) // #nosec:G204, commands executing not vulnerable.
		sanitizedArgs = StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if err != nil {
		return stdout, stderr, fmt.Errorf("an error (%v)"+
			" occurred while running %s args: %v", err, program, sanitizedArgs)
	}

	return stdout, stderr, err
}
