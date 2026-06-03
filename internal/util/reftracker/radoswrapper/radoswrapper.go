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

package radoswrapper

import (
	"github.com/ceph/go-ceph/rados"
)

type (
	IOContext struct {
		*rados.IOContext
	}

	WriteOp struct {
		IoCtx *rados.IOContext
		*rados.WriteOp
	}

	ReadOp struct {
		IoCtx *rados.IOContext
		*rados.ReadOp
	}

	ReadOpOmapGetValsByKeysStep struct {
		*rados.ReadOpOmapGetValsByKeysStep
	}
)

var _ IOContextW = &IOContext{}

func NewIOContext(ioctx *rados.IOContext) IOContextW {
	return &IOContext{
		IOContext: ioctx,
	}
}

func (c *IOContext) GetLastVersion() (uint64, error) {
	return c.IOContext.GetLastVersion()
}

func (c *IOContext) GetXattr(oid, key string, data []byte) (int, error) {
	return c.IOContext.GetXattr(oid, key, data)
}

func (c *IOContext) CreateWriteOp() WriteOpW {
	return &WriteOp{
		IoCtx:   c.IOContext,
		WriteOp: rados.CreateWriteOp(),
	}
}

func (c *IOContext) CreateReadOp() ReadOpW {
	return &ReadOp{
		IoCtx:  c.IOContext,
		ReadOp: rados.CreateReadOp(),
	}
}

func (w *WriteOp) Create(exclusive rados.CreateOption) {
	w.WriteOp.Create(exclusive)
}

func (w *WriteOp) Remove() {
	w.WriteOp.Remove()
}

func (w *WriteOp) SetXattr(name string, value []byte) {
	w.WriteOp.SetXattr(name, value)
}

func (w *WriteOp) WriteFull(b []byte) {
	w.WriteOp.WriteFull(b)
}

func (w *WriteOp) SetOmap(pairs map[string][]byte) {
	w.WriteOp.SetOmap(pairs)
}

func (w *WriteOp) RmOmapKeys(keys []string) {
	w.WriteOp.RmOmapKeys(keys)
}

func (w *WriteOp) AssertVersion(v uint64) {
	w.WriteOp.AssertVersion(v)
}

func (w *WriteOp) Operate(oid string) error {
	return w.WriteOp.Operate(w.IoCtx, oid, rados.OperationNoFlag)
}

func (w *WriteOp) Release() {
	w.WriteOp.Release()
}

func (r *ReadOp) Read(offset uint64, buffer []byte) *rados.ReadOpReadStep {
	return r.ReadOp.Read(offset, buffer)
}

func (r *ReadOp) GetOmapValuesByKeys(keys []string) ReadOpOmapGetValsByKeysStepW {
	return &ReadOpOmapGetValsByKeysStep{
		ReadOpOmapGetValsByKeysStep: r.ReadOp.GetOmapValuesByKeys(keys),
	}
}

func (r *ReadOp) AssertVersion(v uint64) {
	r.ReadOp.AssertVersion(v)
}

func (r *ReadOp) Operate(oid string) error {
	return r.ReadOp.Operate(r.IoCtx, oid, rados.OperationNoFlag)
}

func (r *ReadOp) Release() {
	r.ReadOp.Release()
}

func (s *ReadOpOmapGetValsByKeysStep) Next() (*rados.OmapKeyValue, error) {
	return s.ReadOpOmapGetValsByKeysStep.Next()
}
