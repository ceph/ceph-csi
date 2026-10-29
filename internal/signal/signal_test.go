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

package signal_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// buildCrasher compiles testdata/abort-crasher into a temporary directory and
// returns the path of the resulting binary.
func buildCrasher(t *testing.T) string {
	t.Helper()

	// Locate the go binary via GOROOT so this works inside the containerised
	// test environment where PATH may not include the Go toolchain.
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skipf("go binary not found at %s, skipping: %v", goBin, err)
	}

	// Determine the module root (two levels above this file).
	_, thisFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	crasherDir := filepath.Join(filepath.Dir(thisFile), "testdata", "abort-crasher")

	binaryPath := filepath.Join(t.TempDir(), "abort-crasher")

	buildCmd := exec.Command(goBin, "build", "-mod=vendor", "-o", binaryPath, ".")
	buildCmd.Dir = crasherDir
	buildCmd.Env = append(os.Environ(), "GOFLAGS=-mod=vendor", "GOPATH="+moduleRoot+"/../../..")
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build abort-crasher: %v\n%s", err, out)
	}

	return binaryPath
}

// syncBuffer is a bytes.Buffer that is safe for concurrent use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// runCrasher runs the abort helper binary in the given mode ("raise" or
// "abort") with the given SysProcAttr. If the process does not exit within 10
// seconds, it is killed and the test fails.
//
// When strict is set, the process must exit with status 134 (128 + SIGABRT)
// and print the goroutine dump, which proves that the Handle() signal handler
// ran. Otherwise any termination is accepted: what happens after glibc's
// abort() has returned from the Go signal handler depends on the glibc
// version (a final faulting instruction usually ends the process with
// SIGSEGV), but the process must never hang.
func runCrasher(
	t *testing.T,
	binaryPath, mode string,
	attr *syscall.SysProcAttr,
	skipOnStartFailure, strict bool,
) {
	t.Helper()

	runCmd := exec.Command(binaryPath, mode)
	runCmd.SysProcAttr = attr
	out := &syncBuffer{}
	runCmd.Stdout = out
	runCmd.Stderr = out

	if err := runCmd.Start(); err != nil {
		if skipOnStartFailure {
			t.Skipf("cannot start abort-crasher in this environment: %v", err)
		}
		t.Fatalf("failed to start abort-crasher: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- runCmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		if killErr := runCmd.Process.Kill(); killErr != nil {
			t.Fatalf("abort-crasher did not exit within 10s and could not be killed: %v", killErr)
		}
		// Wait for the process and its output copying to finish before
		// reading the output.
		t.Logf("abort-crasher wait result after kill: %v", <-done)
		t.Fatalf("abort-crasher did not exit within 10s — signal handler likely failed (PID 1 hang):\n%s",
			out.String())
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v\noutput:\n%s", err, out.String())
	}

	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("cannot get WaitStatus: %v", exitErr)
	}

	if !strict {
		t.Logf("abort-crasher terminated: exited=%v code=%d signaled=%v signal=%v",
			ws.Exited(), ws.ExitStatus(), ws.Signaled(), ws.Signal())

		return
	}

	if !ws.Exited() || ws.ExitStatus() != 134 {
		t.Errorf("unexpected exit: exited=%v code=%d signaled=%v signal=%v\noutput:\n%s",
			ws.Exited(), ws.ExitStatus(), ws.Signaled(), ws.Signal(), out.String())
	}

	if !strings.Contains(out.String(), "goroutine dump follows") {
		t.Errorf("goroutine dump missing from output:\n%s", out.String())
	}
}

// TestHandleAbortsOnNativeThread verifies that Handle() catches SIGABRT raised
// by a native (non-Go) POSIX thread and terminates the process through
// os.Exit() with status 134 (128 + SIGABRT), after logging a goroutine dump.
//
// The test compiles testdata/abort-crasher and runs it as a subprocess. The
// helper installs the signal handler and then starts a C pthread that calls
// raise(SIGABRT) after one second, and keeps running.
func TestHandleAbortsOnNativeThread(t *testing.T) {
	t.Parallel()

	runCrasher(t, buildCrasher(t), "raise", nil, false, true)
}

// TestHandleAbortsOnNativeThreadAsPID1 is similar to
// TestHandleAbortsOnNativeThread, but runs the helper as PID 1 of a new PID
// namespace. This reproduces the SIGNAL_UNKILLABLE situation of a container
// without an init process, where re-raising SIGABRT with SIG_DFL results in a
// process that hangs forever.
//
// The "raise" mode must be handled by Handle(), exit with status 134 and log
// the goroutine dump. The "abort" mode calls the real glibc abort() and only
// verifies that the process does not hang.
//
// The test is skipped when the namespaces can not be created (for example
// when unprivileged user namespaces are disabled).
func TestHandleAbortsOnNativeThreadAsPID1(t *testing.T) {
	t.Parallel()

	attr := &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}

	binaryPath := buildCrasher(t)

	for _, mode := range []string{"raise", "abort"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			runCrasher(t, binaryPath, mode, attr, true, mode == "raise")
		})
	}
}
