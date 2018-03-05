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

package cephfs

import "errors"

type volumeOptions struct {
	VolName     string `json:"volName"`
	Monitor     string `json:"monitor"`
	Pool        string `json:"pool"`
	AdminId     string `json:"adminID"`
	AdminSecret string `json:"adminSecret"`
}

func extractOption(dest *string, optionLabel string, options map[string]string) error {
	if opt, ok := options[optionLabel]; !ok {
		return errors.New("Missing required parameter " + optionLabel)
	} else {
		*dest = opt
		return nil
	}
}

func newVolumeOptions(volOptions map[string]string) (*volumeOptions, error) {
	var opts volumeOptions
	// XXX early return - we're not reading credentials from volOptions for now...
	// i'll finish this once ceph-fuse accepts passing credentials through cmd args
	return &opts, nil

	/*
		if err := extractOption(&opts.AdminId, "adminID", volOptions); err != nil {
			return nil, err
		}

		if err := extractOption(&opts.AdminSecret, "adminSecret", volOptions); err != nil {
			return nil, err
		}

		if err := extractOption(&opts.Monitors, "monitors", volOptions); err != nil {
			return nil, err
		}

		if err := extractOption(&opts.Pool, "pool", volOptions); err != nil {
			return nil, err
		}

		return &opts, nil
	*/
}
