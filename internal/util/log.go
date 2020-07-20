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
	"context"
	"fmt"
)

type contextKey string

// CtxKey for context based logging.
var CtxKey = contextKey("ID")

// ReqID for logging request ID.
var ReqID = contextKey("Req-ID")

// Log helps in context based logging.
func Log(ctx context.Context, format string) string {
	id := ctx.Value(CtxKey)
	if id == nil {
		return format
	}
	a := fmt.Sprintf("ID: %v ", id)
	reqID := ctx.Value(ReqID)
	if reqID == nil {
		return a + format
	}
	a += fmt.Sprintf("Req-ID: %v ", reqID)
	return a + format
}
