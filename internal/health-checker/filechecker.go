/*
Copyright 2023 ceph-csi authors.

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

import (
	"errors"
	"os"
	"path"
	"time"
)

type fileChecker struct {
	checker

	// filename contains the filename that is used for checking.
	filename string
}

func newFileChecker(dir string) ConditionChecker {
	fc := &fileChecker{
		filename: path.Join(dir, "csi-volume-condition.ts"),
	}
	fc.initDefaults()

	fc.checker.runChecker = func() {
		fc.isRunning = true

		ticker := time.NewTicker(fc.interval)
		defer ticker.Stop()

		for {
			select {
			case <-fc.commands: // STOP command received
				fc.isRunning = false

				return
			case now := <-ticker.C:
				err := fc.writeTimestamp(now)
				if err != nil {
					fc.mutex.Lock()
					fc.healthy = false
					fc.err = err
					fc.mutex.Unlock()

					continue
				}

				ts, err := fc.readTimestamp()
				if err != nil {
					fc.mutex.Lock()
					fc.healthy = false
					fc.err = err
					fc.mutex.Unlock()

					continue
				}

				// verify that the written timestamp is read back
				if now.Compare(ts) != 0 {
					fc.mutex.Lock()
					fc.healthy = false
					fc.err = errors.New("timestamp read from file does not match what was written")
					fc.mutex.Unlock()

					continue
				}

				// run health check, write a timestamp to a file, read it back
				fc.mutex.Lock()
				fc.healthy = true
				fc.err = nil
				fc.lastUpdate = ts
				fc.mutex.Unlock()
			}
		}
	}

	return fc
}

// readTimestamp reads the JSON formatted timestamp from the file.
func (fc *fileChecker) readTimestamp() (time.Time, error) {
	var ts time.Time

	data, err := os.ReadFile(fc.filename)
	if err != nil {
		return ts, err
	}

	err = ts.UnmarshalJSON(data)

	return ts, err
}

// writeTimestamp writes the timestamp to the file in JSON format.
func (fc *fileChecker) writeTimestamp(ts time.Time) error {
	data, err := ts.MarshalJSON()
	if err != nil {
		return err
	}

	//nolint:gosec // allow reading of the timestamp for debugging
	return os.WriteFile(fc.filename, data, 0o644)
}
