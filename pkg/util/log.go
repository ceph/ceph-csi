/*
Copyright 2018 The Kubernetes Authors.

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

package util

import (
	"flag"
	"os"

	"k8s.io/klog"
)

// InitLogging initializes klog alongside glog
// XXX: This is just a temporary solution till all deps move to klog
func InitLogging() {
	if err := flag.Set("logtostderr", "true"); err != nil {
		klog.Errorf("failed to set logtostderr flag: %v", err)
		os.Exit(1)
	}

	flag.Parse()

	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	// Sync klog flags with glog
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		if f2 := klogFlags.Lookup(f1.Name); f2 != nil {
			f2.Value.Set(f1.Value.String()) // nolint: errcheck, gosec
		}
	})
}
