/*
Copyright 2025 The CephCSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8s

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// cachedSecret is a string representation of a Kubernetes
// secret's data.
type cachedSecret struct {
	data map[string]string
}

// secretCache is a thread safe cache for secrets.
// /!\ The `cache` must be read/modified with the lock held.
type secretCache struct {
	// cache stores secrets identified by their hash.
	// The cacheKey function defines how each hash is generated.
	cache map[string]cachedSecret
	sync.RWMutex
	// A boolean indicating whether the secret watcher is running.
	running atomic.Bool
}

// newSecretCache returns a new instance of secretCache.
func newSecretCache() *secretCache {
	return &secretCache{
		cache: make(map[string]cachedSecret),
	}
}

// cachedSecrets is a local instance of secretCache.
var cachedSecrets = newSecretCache()

// cacheKey is a helper to create sha256 hash based on a secret's name
// and namespace. The hash is then used as an index for secretCache.
func (sc *secretCache) cacheKey(ns, name string) string {
	input := fmt.Sprintf("%s/%s", ns, name)
	h := sha256.Sum256([]byte(input))

	// Only use 16bytes of the hash as key, collisions are astronimically improbable.
	// UUIDv4 is also 128bits and is used as a standard without worries of collision.
	return hex.EncodeToString(h[:16])
}

// startSecretWatcher handles events on Secrets informer and updates
// or invalidates the secretCache accordingly.
func (sc *secretCache) startSecretWatcher(stopCh chan struct{}) error {
	client, err := NewK8sClient()
	if err != nil {
		return fmt.Errorf("failed to connect to Kubernetes: %w", err)
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		// TODO: The sync interval below may need to be made configurable by the user.
		time.Minute*10,
	)

	informer := factory.Core().V1().Secrets().Informer()
	if _, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj any) {
			if newSecret, ok := newObj.(*corev1.Secret); ok {
				_ = cachedSecrets.updateCache(newSecret, false)
			}
		},
		DeleteFunc: func(obj any) {
			if secret, ok := obj.(*corev1.Secret); ok {
				cachedSecrets.deleteFromCache(secret)
			}
		},
	}); err != nil {
		return fmt.Errorf("failed to add event handler for secrets due to: %w", err)
	}

	go informer.Run(stopCh)

	return nil
}

// deleteFromCache deletes a secret from secretCache.
func (sc *secretCache) deleteFromCache(secret *corev1.Secret) {
	key := sc.cacheKey(secret.Namespace, secret.Name)

	sc.Lock()
	defer sc.Unlock()

	delete(sc.cache, key)
}

// updateCache updates a secret in secretCache if it is already present
// or if force it set to true. It returns the secret's data if successful.
func (sc *secretCache) updateCache(secret *corev1.Secret, force bool) map[string]string {
	cKey := sc.cacheKey(secret.Namespace, secret.Name)

	// populate data outside the lock
	secretData := make(map[string]string, len(secret.Data))
	for key, value := range secret.Data {
		secretData[key] = string(value)
	}

	cs := cachedSecret{
		data: secretData,
	}

	// If the key is not already in the cache and force == false, do not cache
	// Otherwise store (or update) the secret in cache and return its data.
	//
	// Note: force is true only when we fetch the secret from the API server.
	// It is set to false when called for all events on the informers.
	sc.Lock()
	defer sc.Unlock()

	if _, present := sc.cache[cKey]; !present && !force {
		return nil
	}

	sc.cache[cKey] = cs

	return cs.data
}

// GetSecret retrieves a Kubernetes Secret by its name and namespace.
// It returns a map of key-value pairs contained in the Secret's data.
// If the Secret does not exist or cannot be accessed, it returns an error.
func GetSecret(secretName, secretNamespace string) (map[string]string, error) {
	client, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes: %w", err)
	}

	secret, err := client.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %q in namespace %q information: %w", secretName, secretNamespace, err)
	}

	secretData := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		secretData[k] = string(v)
	}

	return secretData, nil
}

// FIXME: Implement the secret cache in a manner that does not explode memory.
func _(secretName, secretNamespace string) (map[string]string, error) {
	// Start the watcher if not already running, only once
	if cachedSecrets.running.CompareAndSwap(false, true) {
		stopCh := make(chan struct{})
		if err := cachedSecrets.startSecretWatcher(stopCh); err != nil {
			log.ErrorLogMsg("failed to start secret watcher: %v", err)

			// retry later
			cachedSecrets.running.Store(false)
		} else {
			// Cleanup the channel
			go func() {
				termChan := make(chan os.Signal, 1)
				signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(termChan)

				<-termChan
				close(stopCh)
			}()
		}
	}

	// Read from cache only if watcher is up to avoid
	// returning potentially stale values
	if cachedSecrets.running.Load() {
		key := cachedSecrets.cacheKey(secretNamespace, secretName)

		// Return from cache if present
		cachedSecrets.RLock()
		if entry, found := cachedSecrets.cache[key]; found {
			cachedSecrets.RUnlock()

			return entry.data, nil
		}
		cachedSecrets.RUnlock()
	}

	// The entry was not found in cache, fetch it from the API server
	client, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes: %w", err)
	}

	secret, err := client.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %q in namespace %q information: %w", secretName, secretNamespace, err)
	}

	return cachedSecrets.updateCache(secret, true), nil
}
