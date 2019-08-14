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

	"k8s.io/klog"
)

type Verbosity struct {
	level int
}

func V(i int) *Verbosity {
	return &Verbosity{level: i}
}

type contextKey string

var Key = contextKey("ID")

var id = "ID: %d "

// INFO
func (v *Verbosity) Infof(ctx context.Context, format string, args ...interface{}) {
	format = id + format
	args = append([]interface{}{ctx.Value(Key)}, args...)
	klog.V(klog.Level(v.level)).Infof(format, args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	format = id + format
	args = append([]interface{}{ctx.Value(Key)}, args...)
	klog.Infof(format, args...)
}

func (v *Verbosity) Info(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf(id, ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.V(klog.Level(v.level)).Info(args...)
}

func Info(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf(id, ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Info(args...)
}

func (v *Verbosity) Infoln(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf("ID: %d", ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.V(klog.Level(v.level)).Infoln(args...)
}

func Infoln(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf("ID: %d", ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Infoln(args...)
}

// WARNING
func Warningf(ctx context.Context, format string, args ...interface{}) {
	format = id + format
	args = append([]interface{}{ctx.Value(Key)}, args...)
	klog.Warningf(format, args...)
}

func Warning(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf(id, ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Warning(args...)
}

func Warningln(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf("ID: %d", ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Warningln(args...)
}

// ERROR
func Errorf(ctx context.Context, format string, args ...interface{}) {
	format = id + format
	args = append([]interface{}{ctx.Value(Key)}, args...)
	klog.Errorf(format, args...)
}

func Error(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf(id, ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Error(args...)
}

func Errorln(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf("ID: %d", ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Errorln(args...)
}

// FATAL
func Fatalf(ctx context.Context, format string, args ...interface{}) {
	format = id + format
	args = append([]interface{}{ctx.Value(Key)}, args...)
	klog.Fatalf(format, args...)
}

func Fatal(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf(id, ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Fatal(args...)
}

func Fatalln(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf("ID: %d", ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Fatalln(args...)
}

// EXIT
func Exitf(ctx context.Context, format string, args ...interface{}) {
	format = id + format
	args = append([]interface{}{ctx.Value(Key)}, args...)
	klog.Exitf(format, args...)
}

func Exit(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf(id, ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Exit(args...)
}

func Exitln(ctx context.Context, args ...interface{}) {
	idString := fmt.Sprintf("ID: %d", ctx.Value(Key))
	args = append([]interface{}{idString}, args...)
	klog.Exitln(args...)
}
