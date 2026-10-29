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

// Package signal installs a handler for signals that may be raised by native
// (non-Go) threads such as the Ceph client libraries (librados, librbd,
// libcephfs).
//
// # PID 1 and the SIGNAL_UNKILLABLE problem
//
// When cephcsi runs as PID 1 of its container (the common case without an
// init process or shareProcessNamespace), the Linux kernel marks it
// SIGNAL_UNKILLABLE.  glibc's abort() raises SIGABRT on the calling thread;
// Go's runtime sees a signal on a non-Go thread, re-raises it with SIG_DFL
// via tgkill(), and the kernel silently drops it because the sender and
// receiver are both PID 1 and the disposition is default.  The thread then
// spins forever and the process never terminates (see GitHub issue #6676).
//
// The fix is to handle the signal entirely in Go and call os.Exit() directly,
// which bypasses the signal mechanism and always terminates the process.
package signal

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"k8s.io/klog/v2"
)

// exitTimeout is the maximum time spent on collecting diagnostics before the
// process is terminated unconditionally.
const exitTimeout = 5 * time.Second

// Handle registers a goroutine that catches signals which can be raised by
// native (non-Go/CGo) threads and ensures the process exits even when running
// as PID 1.  It should be called once, as early as possible in main().
//
// Signals handled:
//   - SIGABRT – abort(), ceph_assert(), std::terminate() from Ceph libraries
//   - SIGBUS  – bus error (misaligned / non-existent memory in native code)
//   - SIGFPE  – floating-point / integer exception in native code
//   - SIGSEGV – segmentation fault in native code
//
// SIGTERM, SIGINT and SIGHUP are deliberately not handled here. Those are
// shutdown requests, and are handled by the components that need a graceful
// shutdown (for example the controller-runtime manager). Forcing an exit on
// those signals would race with, and abort, that graceful shutdown.
func Handle() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs,
		syscall.SIGABRT,
		syscall.SIGBUS,
		syscall.SIGFPE,
		syscall.SIGSEGV,
	)

	go func() {
		sig := <-sigs

		// Use os.Exit rather than re-raising the signal.  When
		// cephcsi runs as PID 1 the kernel drops self-sent signals
		// with default disposition (SIGNAL_UNKILLABLE), so a
		// re-raise would be silently ignored and the process would
		// hang.  os.Exit(128 + signal number) follows the
		// conventional shell exit-code encoding, making the cause
		// visible in kubectl describe pod / container status.
		code := 1
		if sysSig, ok := sig.(syscall.Signal); ok {
			code = 128 + int(sysSig)
		}

		// Logging can block (full log pipe, klog mutex held by another
		// goroutine). Start the exit deadline before doing anything else
		// so that diagnostics are strictly best-effort.
		time.AfterFunc(exitTimeout, func() { os.Exit(code) })

		// Capture a full goroutine dump so operators have diagnostics in
		// the container log.
		buf := make([]byte, 1<<20) // 1 MiB
		n := runtime.Stack(buf, true)
		klog.Errorf("received signal %v – goroutine dump follows:\n%s", sig, buf[:n])
		klog.Flush()

		os.Exit(code)
	}()
}
