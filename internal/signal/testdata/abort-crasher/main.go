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

// abort-crasher is a helper binary used by the tests in the signal package.
// It installs the signal handler then spawns a native POSIX thread via CGo
// that crashes after a short delay, simulating a ceph_assert() failure inside
// librados/librbd. The way it crashes is selected with the first argument:
//
//   - "raise" (default): raise(SIGABRT), and keep the thread running. This is
//     what a thread of a PID 1 process does when the kernel drops the
//     re-raised SIGABRT (the hang from issue #6676), and it makes the test
//     deterministic.
//   - "abort": call glibc's abort(), which resets the disposition to SIG_DFL,
//     raises SIGABRT again and finally executes a faulting instruction when
//     the signal did not terminate the process.
package main

/*
#include <pthread.h>
#include <signal.h>
#include <stdlib.h>
#include <unistd.h>

static void *crashthread(void *arg) {
	int use_abort = *(int *)arg;

	sleep(1);
	if (use_abort) {
		abort();
	}
	raise(SIGABRT);
	for (;;) {
		pause();
	}
	return NULL;
}

static int use_abort;

static void start_crashthread(int abort_mode) {
	pthread_t t;

	use_abort = abort_mode;
	pthread_create(&t, NULL, crashthread, &use_abort);
}
*/
import "C"

import (
	"fmt"
	"os"
	"time"

	cephcsisignal "github.com/ceph/ceph-csi/internal/signal"
)

func main() {
	abortMode := 0
	if len(os.Args) > 1 && os.Args[1] == "abort" {
		abortMode = 1
	}

	cephcsisignal.Handle()
	C.start_crashthread(C.int(abortMode))
	time.Sleep(5 * time.Second)
	fmt.Println("still alive — signal handler did not terminate the process")
}
