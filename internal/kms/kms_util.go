/*
Copyright 2022 The Ceph-CSI Authors.

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

package kms

import "fmt"

// setConfigInt fetches a value from a configuration map and converts it to
// a integer.
//
// If the value is not available, *option is not adjusted and
// errConfigOptionMissing is returned.
// In case the value is available, but can not be converted to a string,
// errConfigOptionInvalid is returned.
func setConfigInt(option *int, config map[string]interface{}, key string) error {
	value, ok := config[key]
	if !ok {
		return fmt.Errorf("%w: %s", errConfigOptionMissing, key)
	}

	s, ok := value.(float64)
	if !ok {
		return fmt.Errorf("%w: expected float64 for %q, but got %T",
			errConfigOptionInvalid, key, value)
	}

	*option = int(s)

	return nil
}
