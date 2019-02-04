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

import (
	"fmt"
	"strconv"
)

type volumeOptions struct {
	Monitors string `json:"monitors"`
	Pool     string `json:"pool"`
	RootPath string `json:"rootPath"`

	Mounter         string `json:"mounter"`
	ProvisionVolume bool   `json:"provisionVolume"`

	MonValueFromSecret string `json:"monValueFromSecret"`
}

func validateNonEmptyField(field, fieldName string) error {
	if field == "" {
		return fmt.Errorf("parameter '%s' cannot be empty", fieldName)
	}

	return nil
}

func (o *volumeOptions) validate() error {
	if err := validateNonEmptyField(o.Monitors, "monitors"); err != nil {
		if err = validateNonEmptyField(o.MonValueFromSecret, "monValueFromSecret"); err != nil {
			return err
		}
	}

	if err := validateNonEmptyField(o.RootPath, "rootPath"); err != nil {
		if !o.ProvisionVolume {
			return err
		}
	} else {
		if o.ProvisionVolume {
			return fmt.Errorf("non-empty field rootPath is in conflict with provisionVolume=true")
		}
	}

	if o.ProvisionVolume {
		if err := validateNonEmptyField(o.Pool, "pool"); err != nil {
			return err
		}
	}

	if o.Mounter != "" {
		if err := validateMounter(o.Mounter); err != nil {
			return err
		}
	}

	return nil
}

func extractOption(dest *string, optionLabel string, options map[string]string) error {
	opt, ok := options[optionLabel]
	if !ok {
		return fmt.Errorf("missing required field %s", optionLabel)
	}

	*dest = opt
	return nil
}

func validateMounter(m string) error {
	switch m {
	case volumeMounterFuse:
	case volumeMounterKernel:
	default:
		return fmt.Errorf("unknown mounter '%s'. Valid options are 'fuse' and 'kernel'", m)
	}

	return nil
}

func newVolumeOptions(volOptions, secret map[string]string) (*volumeOptions, error) {
	var (
		opts volumeOptions
		err  error
	)

	// extract mon from secret first
	if err = extractOption(&opts.MonValueFromSecret, "monValueFromSecret", volOptions); err == nil {
		mon := ""
		if mon, err = getMonValFromSecret(secret); err == nil && len(mon) > 0 {
			opts.Monitors = mon
		}
	}
	if len(opts.Monitors) == 0 {
		// if not set in secret, get it from parameter
		if err = extractOption(&opts.Monitors, "monitors", volOptions); err != nil {
			return nil, fmt.Errorf("either monitors or monValueFromSecret should be set")
		}
	}

	if err = extractNewVolOpt(&opts, volOptions); err != nil {
		return nil, err
	}

	if err = opts.validate(); err != nil {
		return nil, err
	}

	return &opts, nil
}

func extractNewVolOpt(opts *volumeOptions, volOpt map[string]string) error {
	var (
		provisionVolumeBool string
		err                 error
	)
	if err = extractOption(&provisionVolumeBool, "provisionVolume", volOpt); err != nil {
		return err
	}

	if opts.ProvisionVolume, err = strconv.ParseBool(provisionVolumeBool); err != nil {
		return fmt.Errorf("failed to parse provisionVolume: %v", err)
	}

	if opts.ProvisionVolume {
		if err = extractOption(&opts.Pool, "pool", volOpt); err != nil {
			return err
		}
	} else {
		if err = extractOption(&opts.RootPath, "rootPath", volOpt); err != nil {
			return err
		}
	}

	// This field is optional, don't check for its presence
	// nolint
	//  (skip errcheck  and gosec as this is optional)
	extractOption(&opts.Mounter, "mounter", volOpt)
	return nil
}
