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
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/ceph/go-ceph/rados"
)

// chunkSize is the number of key-value pairs that will be fetched in
// one call. This is set fairly large to avoid calling into ceph APIs
// over and over.
const chunkSize int64 = 512

func getOMapValues(
	ctx context.Context,
	conn *Connection,
	poolName, namespace, oid, prefix string, keys []string,
) (map[string]string, error) {
	// fetch and configure the rados ioctx
	ioctx, err := conn.conn.GetIoctx(poolName)
	if err != nil {
		return nil, omapPoolError(err)
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
	numKeys := uint64(0)
	startAfter := ""
	for {
		prevNumKeys := numKeys
		err = ioctx.ListOmapValues(
			oid, startAfter, prefix, chunkSize,
			func(key string, value []byte) {
				numKeys++
				startAfter = key
				if want[key] {
					results[key] = string(value)
				}
			},
		)
		// if we hit an error, or no new keys were seen, exit the loop
		if err != nil || numKeys == prevNumKeys {
			break
		}
	}

	if err != nil {
		if errors.Is(err, rados.ErrNotFound) {
			log.ErrorLog(ctx, "omap not found (pool=%q, namespace=%q, name=%q): %v",
				poolName, namespace, oid, err)

			return nil, util.JoinErrors(util.ErrKeyNotFound, err)
		}

		return nil, err
	}

	log.DebugLog(ctx, "got omap values: (pool=%q, namespace=%q, name=%q): %+v",
		poolName, namespace, oid, results)

	return results, nil
}

func removeMapKeys(
	ctx context.Context,
	conn *Connection,
	poolName, namespace, oid string, keys []string,
) error {
	// fetch and configure the rados ioctx
	ioctx, err := conn.conn.GetIoctx(poolName)
	if err != nil {
		return omapPoolError(err)
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
			log.DebugLog(ctx, "when removing omap keys, omap not found (pool=%q, namespace=%q, name=%q): %+v",
				poolName, namespace, oid, keys)
		} else {
			log.ErrorLog(ctx, "failed removing omap keys (pool=%q, namespace=%q, name=%q): %v",
				poolName, namespace, oid, err)

			return err
		}
	}
	log.DebugLog(ctx, "removed omap keys (pool=%q, namespace=%q, name=%q): %+v",
		poolName, namespace, oid, keys)

	return nil
}

func setOMapKeys(
	ctx context.Context,
	conn *Connection,
	poolName, namespace, oid string, pairs map[string]string,
) error {
	// fetch and configure the rados ioctx
	ioctx, err := conn.conn.GetIoctx(poolName)
	if err != nil {
		return omapPoolError(err)
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
		log.ErrorLog(ctx, "failed setting omap keys (pool=%q, namespace=%q, name=%q, pairs=%+v): %v",
			poolName, namespace, oid, pairs, err)

		return err
	}
	log.DebugLog(ctx, "set omap keys (pool=%q, namespace=%q, name=%q): %+v)",
		poolName, namespace, oid, pairs)

	return nil
}

func omapPoolError(err error) error {
	if errors.Is(err, rados.ErrNotFound) {
		return util.JoinErrors(util.ErrPoolNotFound, err)
	}

	return err
}
