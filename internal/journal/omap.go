/*
Copyright 2020 The Ceph-CSI Authors.

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

package journal

import (
	"context"
	"errors"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/ceph/go-ceph/rados"
	klog "k8s.io/klog/v2"
)

// listExcess is the number of false-positive key-value pairs we will
// accept from ceph when getting omap values.
var listExcess = 32

func getOMapValues(
	ctx context.Context,
	conn *Connection,
	poolName, namespace, oid, prefix string, keys []string) (map[string]string, error) {
	// fetch and configure the rados ioctx
	ioctx, err := conn.conn.GetIoctx(poolName)
	if err != nil {
		return nil, omapPoolError(poolName, err)
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	results := map[string]string{}
	// want is our "lookup map" that ensures O(1) checks for keys
	// while iterating, without needing to complicate the caller.
	want := make(map[string]bool, len(keys))
	for i := range keys {
		want[keys[i]] = true
	}

	err = ioctx.ListOmapValues(
		oid, "", prefix, int64(len(want)+listExcess),
		func(key string, value []byte) {
			if want[key] {
				results[key] = string(value)
			}
		},
	)
	if err != nil {
		if errors.Is(err, rados.ErrNotFound) {
			klog.Errorf(
				util.Log(ctx, "omap not found (pool=%q, namespace=%q, name=%q): %v"),
				poolName, namespace, oid, err)
			return nil, util.NewErrKeyNotFound(oid, err)
		}
		return nil, err
	}

	util.DebugLog(ctx, "got omap values: (pool=%q, namespace=%q, name=%q): %+v",
		poolName, namespace, oid, results)
	return results, nil
}

func removeMapKeys(
	ctx context.Context,
	conn *Connection,
	poolName, namespace, oid string, keys []string) error {
	// fetch and configure the rados ioctx
	ioctx, err := conn.conn.GetIoctx(poolName)
	if err != nil {
		return omapPoolError(poolName, err)
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.RmOmapKeys(oid, keys)
	if err != nil {
		if errors.Is(err, rados.ErrNotFound) {
			// the previous implementation of removing omap keys (via the cli)
			// treated failure to find the omap as a non-error. Do so here to
			// mimic the previous behavior.
			util.DebugLog(ctx, "when removing omap keys, omap not found (pool=%q, namespace=%q, name=%q): %+v",
				poolName, namespace, oid, keys)
		} else {
			klog.Errorf(
				util.Log(ctx, "failed removing omap keys (pool=%q, namespace=%q, name=%q): %v"),
				poolName, namespace, oid, err)
			return err
		}
	}
	util.DebugLog(ctx, "removed omap keys (pool=%q, namespace=%q, name=%q): %+v",
		poolName, namespace, oid, keys)
	return nil
}

func setOMapKeys(
	ctx context.Context,
	conn *Connection,
	poolName, namespace, oid string, pairs map[string]string) error {
	// fetch and configure the rados ioctx
	ioctx, err := conn.conn.GetIoctx(poolName)
	if err != nil {
		return omapPoolError(poolName, err)
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	bpairs := make(map[string][]byte, len(pairs))
	for k, v := range pairs {
		bpairs[k] = []byte(v)
	}
	err = ioctx.SetOmap(oid, bpairs)
	if err != nil {
		klog.Errorf(
			util.Log(ctx, "failed setting omap keys (pool=%q, namespace=%q, name=%q, pairs=%+v): %v"),
			poolName, namespace, oid, pairs, err)
		return err
	}
	util.DebugLog(ctx, "set omap keys (pool=%q, namespace=%q, name=%q): %+v)",
		poolName, namespace, oid, pairs)
	return nil
}

func omapPoolError(poolName string, err error) error {
	if errors.Is(err, rados.ErrNotFound) {
		return util.NewErrPoolNotFound(poolName, err)
	}
	return err
}
