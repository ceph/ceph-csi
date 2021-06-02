/*
Copyright 2020 ceph-csi authors.

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
	"errors"
	"fmt"
	"testing"
)

var (
	errFoo = errors.New("foo")
	errBar = errors.New("bar")
)

func wrapError(e error) error {
	return fmt.Errorf("w{%w}", e)
}

func TestJoinErrors(t *testing.T) {
	t.Parallel()
	assertErrorIs := func(e1, e2 error, ok bool) {
		if errors.Is(e1, e2) != ok {
			t.Errorf("errors.Is(e1, e2) != %v - e1: %#v - e2: %#v", ok, e1, e2)
		}
	}

	assertErrorIs(errFoo, errBar, false)
	assertErrorIs(errFoo, errFoo, true)

	fooBar := JoinErrors(errFoo, errBar)
	assertErrorIs(fooBar, errFoo, true)
	assertErrorIs(fooBar, errBar, true)

	w2Foo := wrapError(wrapError(errFoo))
	w1Bar := wrapError(errBar)
	w1w2Foow1Bar := wrapError(JoinErrors(w2Foo, w1Bar))
	assertErrorIs(w1w2Foow1Bar, errFoo, true)
	assertErrorIs(w1w2Foow1Bar, errBar, true)

	w2X := wrapError(wrapError(errors.New("x")))
	w2FooBar := wrapError(wrapError(fooBar))
	w1w2Xw2FooBar := wrapError(JoinErrors(w2X, w2FooBar))
	assertErrorIs(w1w2Xw2FooBar, errFoo, true)
	assertErrorIs(w1w2Xw2FooBar, errBar, true)

	x := errors.Unwrap(errors.Unwrap(errors.Unwrap(errors.Unwrap(w1w2Xw2FooBar))))
	assertErrorIs(x, fooBar, true)
	x = errors.Unwrap(x)
	assertErrorIs(x, fooBar, false)
	assertErrorIs(x, errFoo, false)
	assertErrorIs(x, errBar, true)
	s1 := "w{w{w{x}}: w{w{foo: bar}}}"
	if s2 := w1w2Xw2FooBar.Error(); s1 != s2 {
		t.Errorf("%s != %s", s1, s2)
	}
}
