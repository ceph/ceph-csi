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

// command is what is sent through the channel to terminate the go routine.
type command string

const (
	// stopCommand is sent through the channel to stop checking.
	stopCommand = command("STOP")
)

type fileChecker struct {
	// filename contains the filename that is used for checking.
	filename string

	// interval contains the time to sleep between health checks.
	interval time.Duration

	// current status
	healthy    bool
	err        error
	isRunning  bool
	lastUpdate time.Time

	// commands is the channel to read commands from; when to stop.
	commands chan command
}

func newFileChecker(dir string) ConditionChecker {
	return &fileChecker{
		filename:   path.Join(dir, "csi-volume-condition.ts"),
		healthy:    true,
		interval:   120 * time.Second,
		lastUpdate: time.Now(),
		commands:   make(chan command),
	}
}

// runChecker is an endless loop that writes a timestamp and reads it back from
// a file.
func (fc *fileChecker) runChecker() {
	fc.isRunning = true

	for {
		if fc.shouldStop() {
			fc.isRunning = false

			return
		}

		now := time.Now()

		err := fc.writeTimestamp(now)
		if err != nil {
			fc.healthy = false
			fc.err = err

			continue
		}

		ts, err := fc.readTimestamp()
		if err != nil {
			fc.healthy = false
			fc.err = err

			continue
		}

		// verify that the written timestamp is read back
		if now.Compare(ts) != 0 {
			fc.healthy = false
			fc.err = errors.New("timestamp read from file does not match what was written")

			continue
		}

		// run health check, write a timestamp to a file, read it back
		fc.healthy = true
		fc.err = nil
		fc.lastUpdate = ts
	}
}

func (fc *fileChecker) shouldStop() bool {
	start := time.Now()

	for {
		// check if we slept long enough to run a next check
		slept := time.Since(start)
		if slept >= fc.interval {
			break
		}

		select {
		case <-fc.commands:
			// a command was reveived, need to stop checking
			return true
		default:
			// continue with checking
		}

		time.Sleep(time.Second)
	}

	return false
}

func (fc *fileChecker) start() {
	go fc.runChecker()
}

func (fc *fileChecker) stop() {
	fc.commands <- stopCommand
}

func (fc *fileChecker) isHealthy() (bool, error) {
	// check for the last update, it should be within a certain number of
	//
	//   lastUpdate + (N x fc.interval)
	//
	// Without such a check, a single slow write/read could trigger actions
	// to recover an unhealthy volume already.
	//
	// It is required to check, in case the write or read in the go routine
	// is blocked.
	return fc.healthy, fc.err
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

	return os.WriteFile(fc.filename, data, 0644)
}
