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
	Monitors string `json:"monitors"`
	RootPath string `json:"rootPath"`
	User     string `json:"user"`
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

	if err := extractOption(&opts.Monitors, "monitors", volOptions); err != nil {
		return nil, err
	}

	if err := extractOption(&opts.RootPath, "rootPath", volOptions); err != nil {
		return nil, err
	}

	if err := extractOption(&opts.User, "user", volOptions); err != nil {
		return nil, err
	}

	return &opts, nil
}
