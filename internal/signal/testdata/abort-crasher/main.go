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

// abort-crasher is a helper binary used by TestHandleAbortsOnNativeThread.
// It installs the signal handler then spawns a native POSIX thread via CGo
// that calls abort() after a short delay, simulating a ceph_assert() failure
// inside librados/librbd running as PID 1.
package main

/*
#include <pthread.h>
#include <stdlib.h>
#include <unistd.h>

static void *abortthread(void *arg) {
	sleep(1);
	abort();
	return NULL;
}

static void start_abortthread(void) {
	pthread_t t;
	pthread_create(&t, NULL, abortthread, NULL);
}
*/
import "C"

import (
	"fmt"
	"time"

	cephcsisignal "github.com/ceph/ceph-csi/internal/signal"
)

func main() {
	cephcsisignal.Handle()
	C.start_abortthread()
	time.Sleep(5 * time.Second)
	fmt.Println("still alive — signal handler did not terminate the process")
}
