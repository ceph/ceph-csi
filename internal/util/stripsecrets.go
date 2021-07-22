/*
Copyright 2019 The Ceph-CSI Authors.

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
	"strings"
)

const (
	keyArg              = "--key="
	keyFileArg          = "--keyfile="
	secretArg           = "secret="
	optionsArgSeparator = ','
	strippedKey         = "--key=***stripped***"
	strippedKeyFile     = "--keyfile=***stripped***"
	strippedSecret      = "secret=***stripped***"
)

// StripSecretInArgs strips values of either "--key"/"--keyfile" or "secret=".
// `args` is left unchanged.
// Expects only one occurrence of either "--key"/"--keyfile" or "secret=".
func StripSecretInArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)

	if !stripKey(out) {
		stripSecret(out)
	}

	return out
}

func stripKey(out []string) bool {
	for i := range out {
		if strings.HasPrefix(out[i], keyArg) {
			out[i] = strippedKey

			return true
		}

		if strings.HasPrefix(out[i], keyFileArg) {
			out[i] = strippedKeyFile

			return true
		}
	}

	return false
}

func stripSecret(out []string) bool {
	for i := range out {
		arg := out[i]
		begin := strings.Index(arg, secretArg)

		if begin == -1 {
			continue
		}

		end := strings.IndexByte(arg[begin+len(secretArg):], optionsArgSeparator)

		out[i] = arg[:begin] + strippedSecret
		if end != -1 {
			out[i] += arg[end+len(secretArg):]
		}

		return true
	}

	return false
}
