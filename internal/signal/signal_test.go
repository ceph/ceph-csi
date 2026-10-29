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
	"syscall"
	"testing"
	"time"
)

// TestHandleAbortsOnNativeThread verifies that Handle() catches SIGABRT raised
// by a native (non-Go) POSIX thread and terminates the process via os.Exit(),
// even when the process is PID 1 (SIGNAL_UNKILLABLE).
//
// The test compiles testdata/abort-crasher and runs it as a subprocess.  The
// helper installs the signal handler and then starts a C pthread that calls
// abort() after one second.  We assert the subprocess exits within 10 seconds
// with either:
//   - ExitCode 134 (128 + SIGABRT==6), or
//   - killed by SIGABRT (when NOT running as PID 1, the kernel may deliver it
//     the normal way after Go's signal handler fires os.Exit but before the
//     exit reaches the kernel — both outcomes are acceptable).
func TestHandleAbortsOnNativeThread(t *testing.T) {
	t.Parallel()

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

	// Build the helper binary into a temp directory.
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "abort-crasher")

	buildCmd := exec.Command(goBin, "build", "-mod=vendor", "-o", binaryPath, ".")
	buildCmd.Dir = crasherDir
	buildCmd.Env = append(os.Environ(), "GOFLAGS=-mod=vendor", "GOPATH="+moduleRoot+"/../../..")
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut

	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build abort-crasher: %v\n%s", err, buildOut.String())
	}

	// Run the helper and wait up to 10 s.
	runCmd := exec.Command(binaryPath)
	var runOut bytes.Buffer
	runCmd.Stdout = &runOut
	runCmd.Stderr = &runOut

	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start abort-crasher: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- runCmd.Wait()
	}()

	select {
	case err := <-done:
		// Process finished — check the exit code / signal.
		if err == nil {
			// Printed "still alive" → handler failed.
			t.Errorf("abort-crasher exited 0 (still alive): %s", runOut.String())

			return
		}

		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("unexpected wait error type: %v", err)

			return
		}

		ws, ok := exitErr.Sys().(syscall.WaitStatus)
		if !ok {
			t.Errorf("cannot get WaitStatus: %v", exitErr)

			return
		}

		switch {
		case ws.Exited() && ws.ExitStatus() == 134:
			// os.Exit(128+6) — signal handler fired correctly.
		case ws.Signaled() && ws.Signal() == syscall.SIGABRT:
			// Killed by SIGABRT before os.Exit completed — also fine.
		default:
			t.Errorf("unexpected exit: exited=%v code=%d signaled=%v signal=%v\noutput:\n%s",
				ws.Exited(), ws.ExitStatus(), ws.Signaled(), ws.Signal(), runOut.String())
		}

	case <-time.After(10 * time.Second):
		if err := runCmd.Process.Kill(); err != nil {
			t.Logf("failed to kill hung abort-crasher: %v", err)
		}
		t.Errorf("abort-crasher did not exit within 10s — signal handler likely failed (PID 1 hang):\n%s",
			runOut.String())
	}
}
