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

package log

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"
)

// enum defining logging levels.
const (
	Default klog.Level = iota + 1
	Useful
	Extended
	Debug
	Trace
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

// FatalLog helps in logging fatal errors.
func FatalLogMsg(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	klog.FatalDepth(1, logMessage)
}

// ErrorLogMsg helps in logging errors with message.
func ErrorLogMsg(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	klog.ErrorDepth(1, logMessage)
}

// ErrorLog helps in logging errors with context.
func ErrorLog(ctx context.Context, message string, args ...interface{}) {
	logMessage := fmt.Sprintf(Log(ctx, message), args...)
	klog.ErrorDepth(1, logMessage)
}

// WarningLogMsg helps in logging warnings with message.
func WarningLogMsg(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	klog.WarningDepth(1, logMessage)
}

// WarningLog helps in logging warnings with context.
func WarningLog(ctx context.Context, message string, args ...interface{}) {
	logMessage := fmt.Sprintf(Log(ctx, message), args...)
	klog.WarningDepth(1, logMessage)
}

// DefaultLog helps in logging with klog.level 1.
func DefaultLog(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Default).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// UsefulLog helps in logging with klog.level 2.
func UsefulLog(ctx context.Context, message string, args ...interface{}) {
	logMessage := fmt.Sprintf(Log(ctx, message), args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Useful).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// ExtendedLogMsg helps in logging a message with klog.level 3.
func ExtendedLogMsg(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Extended).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// ExtendedLog helps in logging with klog.level 3.
func ExtendedLog(ctx context.Context, message string, args ...interface{}) {
	logMessage := fmt.Sprintf(Log(ctx, message), args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Extended).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// DebugLogMsg helps in logging a message with klog.level 4.
func DebugLogMsg(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Debug).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// DebugLog helps in logging with klog.level 4.
func DebugLog(ctx context.Context, message string, args ...interface{}) {
	logMessage := fmt.Sprintf(Log(ctx, message), args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Debug).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// TraceLogMsg helps in logging a message with klog.level 5.
func TraceLogMsg(message string, args ...interface{}) {
	logMessage := fmt.Sprintf(message, args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Trace).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}

// TraceLog helps in logging with klog.level 5.
func TraceLog(ctx context.Context, message string, args ...interface{}) {
	logMessage := fmt.Sprintf(Log(ctx, message), args...)
	// If logging is disabled, don't evaluate the arguments
	if klog.V(Trace).Enabled() {
		klog.InfoDepth(1, logMessage)
	}
}
