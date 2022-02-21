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
	"fmt"

	"github.com/ceph/go-ceph/rados"
	"golang.org/x/sys/unix"
)

type (
	FakeObj struct {
		Oid    string
		Ver    uint64
		Xattrs map[string][]byte
		Omap   map[string][]byte
		Data   []byte
	}

	FakeRados struct {
		Objs map[string]*FakeObj
	}

	FakeIOContext struct {
		LastObjVersion uint64
		Rados          *FakeRados
	}

	FakeWriteOp struct {
		IoCtx *FakeIOContext

		steps map[fakeWriteOpStepExecutorIdx]fakeWriteOpStepExecutor
		oid   string
	}

	FakeReadOp struct {
		IoCtx *FakeIOContext

		steps map[fakeReadOpStepExecutorIdx]fakeReadOpStepExecutor
		oid   string
	}

	fakeWriteOpStepExecutorIdx int
	fakeReadOpStepExecutorIdx  int

	fakeWriteOpStepExecutor interface {
		operate(w *FakeWriteOp) error
	}

	fakeReadOpStepExecutor interface {
		operate(r *FakeReadOp) error
	}

	fakeRadosError int
)

const (
	fakeWriteOpAssertVersionExecutorIdx fakeWriteOpStepExecutorIdx = iota
	fakeWriteOpRemoveExecutorIdx
	fakeWriteOpCreateExecutorIdx
	fakeWriteOpSetXattrExecutorIdx
	fakeWriteOpWriteFullExecutorIdx
	fakeWriteOpRmOmapKeysExecutorIdx
	fakeWriteOpSetOmapExecutorIdx

	fakeReadOpAssertVersionExecutorIdx fakeReadOpStepExecutorIdx = iota
	fakeReadOpReadExecutorIdx
	fakeReadOpGetOmapValuesByKeysExecutorIdx
)

var (
	_ IOContextW = &FakeIOContext{}

	// fakeWriteOpStepExecutorOrder defines fixed order in which the write ops are performed.
	fakeWriteOpStepExecutorOrder = []fakeWriteOpStepExecutorIdx{
		fakeWriteOpAssertVersionExecutorIdx,
		fakeWriteOpRemoveExecutorIdx,
		fakeWriteOpCreateExecutorIdx,
		fakeWriteOpSetXattrExecutorIdx,
		fakeWriteOpWriteFullExecutorIdx,
		fakeWriteOpRmOmapKeysExecutorIdx,
		fakeWriteOpSetOmapExecutorIdx,
	}

	// fakeReadOpStepExecutorOrder defines fixed order in which the read ops are performed.
	fakeReadOpStepExecutorOrder = []fakeReadOpStepExecutorIdx{
		fakeReadOpAssertVersionExecutorIdx,
		fakeReadOpReadExecutorIdx,
		fakeReadOpGetOmapValuesByKeysExecutorIdx,
	}
)

func NewFakeRados() *FakeRados {
	return &FakeRados{
		Objs: make(map[string]*FakeObj),
	}
}

func NewFakeIOContext(fakeRados *FakeRados) *FakeIOContext {
	return &FakeIOContext{
		Rados: fakeRados,
	}
}

func (e fakeRadosError) Error() string {
	return fmt.Sprintf("FakeRados errno=%d", int(e))
}

func (e fakeRadosError) ErrorCode() int {
	return int(e)
}

func (o *FakeObj) String() string {
	return fmt.Sprintf("%s{Ver=%d, Xattrs(%d)=%+v, OMap(%d)=%+v, Data(%d)=%+v}",
		o.Oid, o.Ver, len(o.Xattrs), o.Xattrs, len(o.Omap), o.Omap, len(o.Data), o.Data)
}

func (c *FakeIOContext) GetLastVersion() (uint64, error) {
	return c.LastObjVersion, nil
}

func (c *FakeIOContext) getObj(oid string) (*FakeObj, error) {
	obj, ok := c.Rados.Objs[oid]
	if !ok {
		return nil, rados.ErrNotFound
	}

	return obj, nil
}

func (c *FakeIOContext) GetXattr(oid, key string, data []byte) (int, error) {
	obj, ok := c.Rados.Objs[oid]
	if !ok {
		return 0, rados.ErrNotFound
	}

	xattr, ok := obj.Xattrs[key]
	if !ok {
		return 0, fakeRadosError(-int(unix.ENODATA))
	}
	copy(data, xattr)

	return len(xattr), nil
}

func (c *FakeIOContext) CreateWriteOp() WriteOpW {
	return &FakeWriteOp{
		IoCtx: c,
		steps: make(map[fakeWriteOpStepExecutorIdx]fakeWriteOpStepExecutor),
	}
}

func (w *FakeWriteOp) Operate(oid string) error {
	if len(w.steps) == 0 {
		return nil
	}

	w.oid = oid

	for _, writeOpExecutorIdx := range fakeWriteOpStepExecutorOrder {
		e, ok := w.steps[writeOpExecutorIdx]
		if !ok {
			continue
		}

		if err := e.operate(w); err != nil {
			return err
		}
	}

	if obj, err := w.IoCtx.getObj(oid); err == nil {
		obj.Ver++
		w.IoCtx.LastObjVersion = obj.Ver
	}

	return nil
}

func (w *FakeWriteOp) Release() {}

func (c *FakeIOContext) CreateReadOp() ReadOpW {
	return &FakeReadOp{
		IoCtx: c,
		steps: make(map[fakeReadOpStepExecutorIdx]fakeReadOpStepExecutor),
	}
}

func (r *FakeReadOp) Operate(oid string) error {
	r.oid = oid

	for _, readOpExecutorIdx := range fakeReadOpStepExecutorOrder {
		e, ok := r.steps[readOpExecutorIdx]
		if !ok {
			continue
		}

		if err := e.operate(r); err != nil {
			return err
		}
	}

	if obj, err := r.IoCtx.getObj(oid); err == nil {
		r.IoCtx.LastObjVersion = obj.Ver
	}

	return nil
}

func (r *FakeReadOp) Release() {}

// WriteOp Create

type fakeWriteOpCreateExecutor struct {
	exclusive rados.CreateOption
}

func (e *fakeWriteOpCreateExecutor) operate(w *FakeWriteOp) error {
	if e.exclusive == rados.CreateExclusive {
		if _, exists := w.IoCtx.Rados.Objs[w.oid]; exists {
			return rados.ErrObjectExists
		}
	}

	w.IoCtx.Rados.Objs[w.oid] = &FakeObj{
		Oid:    w.oid,
		Omap:   make(map[string][]byte),
		Xattrs: make(map[string][]byte),
	}

	return nil
}

func (w *FakeWriteOp) Create(exclusive rados.CreateOption) {
	w.steps[fakeWriteOpCreateExecutorIdx] = &fakeWriteOpCreateExecutor{
		exclusive: exclusive,
	}
}

// WriteOp Remove

type fakeWriteOpRemoveExecutor struct{}

func (e *fakeWriteOpRemoveExecutor) operate(w *FakeWriteOp) error {
	if _, err := w.IoCtx.getObj(w.oid); err != nil {
		return err
	}

	delete(w.IoCtx.Rados.Objs, w.oid)

	return nil
}

func (w *FakeWriteOp) Remove() {
	w.steps[fakeWriteOpRemoveExecutorIdx] = &fakeWriteOpRemoveExecutor{}
}

// WriteOp SetXattr

type fakeWriteOpSetXattrExecutor struct {
	name  string
	value []byte
}

func (e *fakeWriteOpSetXattrExecutor) operate(w *FakeWriteOp) error {
	obj, err := w.IoCtx.getObj(w.oid)
	if err != nil {
		return err
	}

	obj.Xattrs[e.name] = e.value

	return nil
}

func (w *FakeWriteOp) SetXattr(name string, value []byte) {
	valueCopy := append([]byte(nil), value...)

	w.steps[fakeWriteOpSetXattrExecutorIdx] = &fakeWriteOpSetXattrExecutor{
		name:  name,
		value: valueCopy,
	}
}

// WriteOp WriteFull

type fakeWriteOpWriteFullExecutor struct {
	data []byte
}

func (e *fakeWriteOpWriteFullExecutor) operate(w *FakeWriteOp) error {
	obj, err := w.IoCtx.getObj(w.oid)
	if err != nil {
		return err
	}

	obj.Data = e.data

	return nil
}

func (w *FakeWriteOp) WriteFull(b []byte) {
	bCopy := append([]byte(nil), b...)

	w.steps[fakeWriteOpWriteFullExecutorIdx] = &fakeWriteOpWriteFullExecutor{
		data: bCopy,
	}
}

// WriteOp SetOmap

type fakeWriteOpSetOmapExecutor struct {
	pairs map[string][]byte
}

func (e *fakeWriteOpSetOmapExecutor) operate(w *FakeWriteOp) error {
	obj, err := w.IoCtx.getObj(w.oid)
	if err != nil {
		return err
	}

	for k, v := range e.pairs {
		obj.Omap[k] = v
	}

	return nil
}

func (w *FakeWriteOp) SetOmap(pairs map[string][]byte) {
	pairsCopy := make(map[string][]byte, len(pairs))
	for k, v := range pairs {
		vCopy := append([]byte(nil), v...)
		pairsCopy[k] = vCopy
	}

	w.steps[fakeWriteOpSetOmapExecutorIdx] = &fakeWriteOpSetOmapExecutor{
		pairs: pairsCopy,
	}
}

// WriteOp RmOmapKeys

type fakeWriteOpRmOmapKeysExecutor struct {
	keys []string
}

func (e *fakeWriteOpRmOmapKeysExecutor) operate(w *FakeWriteOp) error {
	obj, err := w.IoCtx.getObj(w.oid)
	if err != nil {
		return err
	}

	for _, k := range e.keys {
		delete(obj.Omap, k)
	}

	return nil
}

func (w *FakeWriteOp) RmOmapKeys(keys []string) {
	keysCopy := append([]string(nil), keys...)

	w.steps[fakeWriteOpRmOmapKeysExecutorIdx] = &fakeWriteOpRmOmapKeysExecutor{
		keys: keysCopy,
	}
}

// WriteOp AssertVersion

type fakeWriteOpAssertVersionExecutor struct {
	version uint64
}

func (e *fakeWriteOpAssertVersionExecutor) operate(w *FakeWriteOp) error {
	obj, err := w.IoCtx.getObj(w.oid)
	if err != nil {
		return err
	}

	return validateObjVersion(obj.Ver, e.version)
}

func (w *FakeWriteOp) AssertVersion(v uint64) {
	w.steps[fakeWriteOpAssertVersionExecutorIdx] = &fakeWriteOpAssertVersionExecutor{
		version: v,
	}
}

// ReadOp Read

type fakeReadOpReadExecutor struct {
	offset int
	buffer []byte
	step   *rados.ReadOpReadStep
}

func (e *fakeReadOpReadExecutor) operate(r *FakeReadOp) error {
	obj, err := r.IoCtx.getObj(r.oid)
	if err != nil {
		return err
	}

	if e.offset > len(obj.Data) {
		// RADOS just returns zero bytes read.
		return nil
	}

	end := e.offset + len(e.buffer)
	if end > len(obj.Data) {
		end = len(obj.Data)
	}

	nbytes := end - e.offset
	e.step.BytesRead = int64(nbytes)
	copy(e.buffer, obj.Data[e.offset:])

	return nil
}

func (r *FakeReadOp) Read(offset uint64, buffer []byte) *rados.ReadOpReadStep {
	s := &rados.ReadOpReadStep{}
	r.steps[fakeReadOpReadExecutorIdx] = &fakeReadOpReadExecutor{
		offset: int(offset),
		buffer: buffer,
		step:   s,
	}

	return s
}

// ReadOp GetOmapValuesByKeys

type (
	fakeReadOpGetOmapValuesByKeysExecutor struct {
		keys []string
		step *FakeReadOpOmapGetValsByKeysStep
	}

	FakeReadOpOmapGetValsByKeysStep struct {
		pairs      []rados.OmapKeyValue
		idx        int
		canIterate bool
	}
)

func (e *fakeReadOpGetOmapValuesByKeysExecutor) operate(r *FakeReadOp) error {
	obj, err := r.IoCtx.getObj(r.oid)
	if err != nil {
		return err
	}

	var pairs []rados.OmapKeyValue
	for _, key := range e.keys {
		val, ok := obj.Omap[key]
		if !ok {
			continue
		}

		pairs = append(pairs, rados.OmapKeyValue{
			Key:   key,
			Value: val,
		})
	}

	e.step.pairs = pairs
	e.step.canIterate = true

	return nil
}

func (s *FakeReadOpOmapGetValsByKeysStep) Next() (*rados.OmapKeyValue, error) {
	if !s.canIterate {
		return nil, rados.ErrOperationIncomplete
	}

	if s.idx >= len(s.pairs) {
		return nil, nil
	}

	omapKeyValue := &s.pairs[s.idx]
	s.idx++

	return omapKeyValue, nil
}

func (r *FakeReadOp) GetOmapValuesByKeys(keys []string) ReadOpOmapGetValsByKeysStepW {
	keysCopy := append([]string(nil), keys...)

	s := &FakeReadOpOmapGetValsByKeysStep{}
	r.steps[fakeReadOpGetOmapValuesByKeysExecutorIdx] = &fakeReadOpGetOmapValuesByKeysExecutor{
		keys: keysCopy,
		step: s,
	}

	return s
}

// ReadOp AssertVersion

type fakeReadOpAssertVersionExecutor struct {
	version uint64
}

func (e *fakeReadOpAssertVersionExecutor) operate(r *FakeReadOp) error {
	obj, err := r.IoCtx.getObj(r.oid)
	if err != nil {
		return err
	}

	return validateObjVersion(obj.Ver, e.version)
}

func (r *FakeReadOp) AssertVersion(v uint64) {
	r.steps[fakeReadOpAssertVersionExecutorIdx] = &fakeReadOpAssertVersionExecutor{
		version: v,
	}
}

func validateObjVersion(expected, actual uint64) error {
	// See librados docs for returning error codes in rados_*_op_assert_version:
	// https://docs.ceph.com/en/latest/rados/api/librados/?#c.rados_write_op_assert_version
	// https://docs.ceph.com/en/latest/rados/api/librados/?#c.rados_read_op_assert_version

	if expected > actual {
		return rados.OperationError{
			OpError: fakeRadosError(-int(unix.ERANGE)),
		}
	}

	if expected < actual {
		return rados.OperationError{
			OpError: fakeRadosError(-int(unix.EOVERFLOW)),
		}
	}

	return nil
}
