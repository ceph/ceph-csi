/*
Copyright 2026 The Ceph-CSI Authors.

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

package nvmeof

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// controllerResolver matches the signature of getControllersForDevice, injected
// into the disconnect logic so tests can stub `nvme list-subsys` results.
type controllerResolver func(context.Context, string) ([]string, error)

func TestHasSharedControllers(t *testing.T) {
	t.Parallel()

	// deviceControllers models `nvme list-subsys <device>`. It reflects the real
	// NVMe-oF native multipath naming: namespace block devices are named after the
	// subsystem head instance (nvme0nX), while the fabric controller is a separate
	// instance (nvme1).
	deviceControllers := map[string][]string{
		"/dev/nvme0n1": {"nvme1"},
		"/dev/nvme0n2": {"nvme1"},
		"/dev/nvme0n7": {"nvme1"},
		"/dev/nvme0n9": {"nvme2"}, // served by a different controller
	}

	var resolver controllerResolver = func(_ context.Context, dev string) ([]string, error) {
		return deviceControllers[dev], nil
	}

	tests := []struct {
		name           string
		currentDevPath string
		ctrlSet        map[string]struct{}
		mountedDevices map[string]string
		want           bool
	}{
		{
			// Regression: unstaging /dev/nvme0n7 while other volumes are still
			// mounted on the same controller (nvme1) must NOT disconnect it.
			// The old code reconstructed /dev/nvme1nX names that never matched
			// the mounted /dev/nvme0nX devices and wrongly disconnected nvme1,
			// yanking the block devices out from under the still-mounted volumes.
			name:           "other device shares controller - keep connected",
			currentDevPath: "/dev/nvme0n7",
			ctrlSet:        map[string]struct{}{"nvme1": {}},
			mountedDevices: map[string]string{
				"/dev/nvme0n7": "/staging/n7",
				"/dev/nvme0n1": "/staging/n1",
				"/dev/nvme0n2": "/staging/n2",
			},
			want: true,
		},
		{
			name:           "only current device mounted - safe to disconnect",
			currentDevPath: "/dev/nvme0n7",
			ctrlSet:        map[string]struct{}{"nvme1": {}},
			mountedDevices: map[string]string{
				"/dev/nvme0n7": "/staging/n7",
			},
			want: false,
		},
		{
			name:           "other device on different controller - safe to disconnect",
			currentDevPath: "/dev/nvme0n7",
			ctrlSet:        map[string]struct{}{"nvme1": {}},
			mountedDevices: map[string]string{
				"/dev/nvme0n7": "/staging/n7",
				"/dev/nvme0n9": "/staging/n9", // on nvme2
			},
			want: false,
		},
		{
			name:           "no mounted devices - safe to disconnect",
			currentDevPath: "/dev/nvme0n7",
			ctrlSet:        map[string]struct{}{"nvme1": {}},
			mountedDevices: map[string]string{},
			want:           false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := hasSharedControllers(context.TODO(), tc.currentDevPath,
				tc.ctrlSet, tc.mountedDevices, resolver)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDisconnectIfLastMount(t *testing.T) {
	t.Parallel()

	// deviceControllers models `nvme list-subsys <device>`: all namespaces are
	// served by controller nvme1 (single fabric path), the state seen in the bug.
	deviceControllers := map[string][]string{
		"/dev/nvme0n1": {"nvme1"},
		"/dev/nvme0n2": {"nvme1"},
		"/dev/nvme0n7": {"nvme1"},
	}
	var resolver controllerResolver = func(_ context.Context, dev string) ([]string, error) {
		return deviceControllers[dev], nil
	}

	t.Run("regression: keeps shared controller connected", func(t *testing.T) {
		t.Parallel()
		var disconnected []string
		disconnect := func(_ context.Context, ctrl string) error {
			disconnected = append(disconnected, ctrl)

			return nil
		}

		// Unstage /dev/nvme0n7 while n1 and n2 are still mounted on nvme1.
		err := disconnectIfLastMount(context.TODO(), "/dev/nvme0n7",
			map[string]string{
				"/dev/nvme0n7": "/staging/n7",
				"/dev/nvme0n1": "/staging/n1",
				"/dev/nvme0n2": "/staging/n2",
			},
			resolver, disconnect)
		require.NoError(t, err)
		require.Empty(t, disconnected, "controller still in use must not be disconnected")
	})

	t.Run("disconnects controller when last mount", func(t *testing.T) {
		t.Parallel()
		var disconnected []string
		disconnect := func(_ context.Context, ctrl string) error {
			disconnected = append(disconnected, ctrl)

			return nil
		}

		err := disconnectIfLastMount(context.TODO(), "/dev/nvme0n7",
			map[string]string{"/dev/nvme0n7": "/staging/n7"},
			resolver, disconnect)
		require.NoError(t, err)
		require.Equal(t, []string{"nvme1"}, disconnected)
	})

	t.Run("no-op when device already disconnected", func(t *testing.T) {
		t.Parallel()
		emptyResolver := func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		}
		disconnect := func(_ context.Context, _ string) error {
			t.Fatalf("disconnect must not be called for an already-disconnected device")

			return nil
		}

		err := disconnectIfLastMount(context.TODO(), "/dev/nvme0n7",
			map[string]string{"/dev/nvme0n7": "/staging/n7"},
			emptyResolver, disconnect)
		require.NoError(t, err)
	})

	t.Run("skips disconnect when controller check fails", func(t *testing.T) {
		t.Parallel()
		// Resolving the current device succeeds, but resolving the other mounted
		// device fails - we must not disconnect when we cannot verify safety.
		failingResolver := func(_ context.Context, dev string) ([]string, error) {
			if dev == "/dev/nvme0n7" {
				return []string{"nvme1"}, nil
			}

			return nil, errors.New("list-subsys failed")
		}
		disconnect := func(_ context.Context, _ string) error {
			t.Fatalf("disconnect must not be called when safety cannot be verified")

			return nil
		}

		err := disconnectIfLastMount(context.TODO(), "/dev/nvme0n7",
			map[string]string{
				"/dev/nvme0n7": "/staging/n7",
				"/dev/nvme0n1": "/staging/n1",
			},
			failingResolver, disconnect)
		require.NoError(t, err)
	})

	t.Run("returns error when disconnect fails", func(t *testing.T) {
		t.Parallel()
		disconnect := func(_ context.Context, _ string) error {
			return errors.New("disconnect failed")
		}

		err := disconnectIfLastMount(context.TODO(), "/dev/nvme0n7",
			map[string]string{"/dev/nvme0n7": "/staging/n7"},
			resolver, disconnect)
		require.Error(t, err)
	})
}

func TestHasSharedControllers_ResolverError(t *testing.T) {
	t.Parallel()

	resolver := func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("list-subsys failed")
	}

	_, err := hasSharedControllers(context.TODO(), "/dev/nvme0n7",
		map[string]struct{}{"nvme1": {}},
		map[string]string{
			"/dev/nvme0n7": "/staging/n7",
			"/dev/nvme0n1": "/staging/n1",
		},
		resolver)
	require.Error(t, err)
}

func TestHasSharedControllers_NoControllersIsFailure(t *testing.T) {
	t.Parallel()

	// A device that is supposedly mounted but resolves to zero controllers is
	// anomalous (e.g. a transient list-subsys race), so it must be treated as a
	// verification failure rather than "no overlap".
	var resolver controllerResolver = func(_ context.Context, _ string) ([]string, error) {
		return nil, nil
	}

	_, err := hasSharedControllers(context.TODO(), "/dev/nvme0n7",
		map[string]struct{}{"nvme1": {}},
		map[string]string{
			"/dev/nvme0n7": "/staging/n7",
			"/dev/nvme0n1": "/staging/n1",
		},
		resolver)
	require.Error(t, err)
}
