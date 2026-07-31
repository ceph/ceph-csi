/*
Copyright 2026 ceph-csi authors.

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

package healthchecker

type noOpChecker struct {
	checker
}

func newNoOpChecker() ConditionChecker {
	nc := &noOpChecker{}
	nc.initDefaults()

	// always healthy, no errors
	nc.healthy = true
	nc.err = nil

	nc.checker.runChecker = func() {
		nc.isRunning = true

		// Wait for stop command
		<-nc.commands
		nc.isRunning = false
	}

	return nc
}
