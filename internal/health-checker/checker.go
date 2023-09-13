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
	"fmt"
	"sync"
	"time"
)

// command is what is sent through the channel to terminate the go routine.
type command string

const (
	// stopCommand is sent through the channel to stop checking.
	stopCommand = command("STOP")
)

type checker struct {
	// interval contains the time to sleep between health checks.
	interval time.Duration

	// timeout contains the delay (interval + timeout)
	timeout time.Duration

	// mutex protects against concurrent access to healty, err and
	// lastUpdate
	mutex *sync.RWMutex

	// current status
	isRunning  bool
	healthy    bool
	err        error
	lastUpdate time.Time

	// commands is the channel to read commands from; when to stop.
	commands chan command

	runChecker func()
}

func (c *checker) initDefaults() {
	c.interval = 60 * time.Second
	c.timeout = 15 * time.Second
	c.mutex = &sync.RWMutex{}
	c.isRunning = false
	c.err = nil
	c.healthy = true
	c.lastUpdate = time.Now()
	c.commands = make(chan command)

	c.runChecker = func() {
		panic("BUG: implement runChecker() in the final checker struct")
	}
}

func (c *checker) start() {
	if c.isRunning {
		return
	}

	go c.runChecker()
}

func (c *checker) stop() {
	c.commands <- stopCommand
}

func (c *checker) isHealthy() (bool, error) {
	// check for the last update, it should be within
	//
	//   c.lastUpdate < (c.interval + c.timeout)
	//
	// Without such a check, a single slow write/read could trigger actions
	// to recover an unhealthy volume already.
	//
	// It is required to check, in case the write or read in the go routine
	// is blocked.

	delay := time.Since(c.lastUpdate)
	if delay > (c.interval + c.timeout) {
		c.mutex.Lock()
		c.healthy = false
		c.err = fmt.Errorf("health-check has not responded for %f seconds", delay.Seconds())
		c.mutex.Unlock()
	}

	// read lock to get consistency between the return values
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.healthy, c.err
}
