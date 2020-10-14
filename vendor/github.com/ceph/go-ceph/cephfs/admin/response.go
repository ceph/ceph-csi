// +build !luminous,!mimic

package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrStatusNotEmpty may be returned if a call should not have a status
	// string set but one is.
	ErrStatusNotEmpty = errors.New("response status not empty")
	// ErrBodyNotEmpty may be returned if a call should have an empty body but
	// a body value is present.
	ErrBodyNotEmpty = errors.New("response body not empty")
)

const (
	deprecatedSuffix = "call is deprecated and will be removed in a future release"
	missingPrefix    = "No handler found"
	einval           = -22
)

type cephError interface {
	ErrorCode() int
}

// NotImplementedError error values will be returned in the case that an API
// call is not available in the version of Ceph that is running in the target
// cluster.
type NotImplementedError struct {
	response
}

// Error implements the error interface.
func (e NotImplementedError) Error() string {
	return fmt.Sprintf("API call not implemented server-side: %s", e.status)
}

// response encapsulates the data returned by ceph and supports easy processing
// pipelines.
type response struct {
	body   []byte
	status string
	err    error
}

// Ok returns true if the response contains no error.
func (r response) Ok() bool {
	return r.err == nil
}

// Error implements the error interface.
func (r response) Error() string {
	if r.status == "" {
		return r.err.Error()
	}
	return fmt.Sprintf("%s: %q", r.err, r.status)
}

// Unwrap returns the error this response contains.
func (r response) Unwrap() error {
	return r.err
}

// Status returns the status string value.
func (r response) Status() string {
	return r.status
}

// End returns an error if the response contains an error or nil, indicating
// that response is no longer needed for processing.
func (r response) End() error {
	if !r.Ok() {
		if ce, ok := r.err.(cephError); ok {
			if ce.ErrorCode() == einval && strings.HasPrefix(r.status, missingPrefix) {
				return NotImplementedError{response: r}
			}
		}
		return r
	}
	return nil
}

// noStatus asserts that the input response has no status value.
func (r response) noStatus() response {
	if !r.Ok() {
		return r
	}
	if r.status != "" {
		return response{r.body, r.status, ErrStatusNotEmpty}
	}
	return r
}

// noBody asserts that the input response has no body value.
func (r response) noBody() response {
	if !r.Ok() {
		return r
	}
	if len(r.body) != 0 {
		return response{r.body, r.status, ErrBodyNotEmpty}
	}
	return r
}

// noData asserts that the input response has no status or body values.
func (r response) noData() response {
	return r.noStatus().noBody()
}

// filterDeprecated removes deprecation warnings from the response status.
// Use it when checking the response from calls that may be deprecated in ceph
// if you want those calls to continue working if the warning is present.
func (r response) filterDeprecated() response {
	if !r.Ok() {
		return r
	}
	if strings.HasSuffix(r.status, deprecatedSuffix) {
		return response{r.body, "", r.err}
	}
	return r
}

// unmarshal data from the response body into v.
func (r response) unmarshal(v interface{}) response {
	if !r.Ok() {
		return r
	}
	if err := json.Unmarshal(r.body, v); err != nil {
		return response{body: r.body, err: err}
	}
	return r
}

// newResponse returns a response.
func newResponse(b []byte, s string, e error) response {
	return response{b, s, e}
}
