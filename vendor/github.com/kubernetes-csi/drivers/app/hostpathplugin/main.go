/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"flag"
	"os"

	"github.com/kubernetes-csi/drivers/pkg/hostpath"
)

func init() {
	flag.Set("logtostderr", "true")
}

var (
	endpoint   = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName = flag.String("drivername", "csi-hostpath", "name of the driver")
	nodeID     = flag.String("nodeid", "", "node id")
)

func main() {
	flag.Parse()

	handle()
	os.Exit(0)
}

func handle() {
	driver := hostpath.GetHostPathDriver()
	driver.Run(*driverName, *nodeID, *endpoint)
}
