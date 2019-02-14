/*
Copyright 2018 The Kubernetes Authors.

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
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog"
)

// NodeCache to store metadata
type NodeCache struct {
	BasePath string
}

var cacheDir = "controller"

var errDec = errors.New("file not found")

// EnsureCacheDirectory creates cache directory if not present
func (nc *NodeCache) EnsureCacheDirectory(cacheDir string) error {
	fullPath := path.Join(nc.BasePath, cacheDir)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// #nosec
		if err := os.Mkdir(fullPath, 0755); err != nil {
			return errors.Wrapf(err, "node-cache: failed to create %s folder with error: %v", fullPath, err)
		}
	}
	return nil
}

//ForAll list the metadata in Nodecache and filters outs based on the pattern
func (nc *NodeCache) ForAll(pattern string, destObj interface{}, f ForAllFunc) error {
	err := nc.EnsureCacheDirectory(cacheDir)
	if err != nil {
		return errors.Wrap(err, "node-cache: couldn't ensure cache directory exists")
	}
	files, err := ioutil.ReadDir(path.Join(nc.BasePath, cacheDir))
	if err != nil {
		return errors.Wrapf(err, "node-cache: failed to read %s folder", nc.BasePath)
	}
	path := path.Join(nc.BasePath, cacheDir)
	for _, file := range files {
		err = decodeObj(path, pattern, file, destObj)
		if err == errDec {
			continue
		} else if err == nil {
			if err = f(strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))); err != nil {
				return err
			}
		}
		return err

	}
	return nil
}

func decodeObj(filepath, pattern string, file os.FileInfo, destObj interface{}) error {
	match, err := regexp.MatchString(pattern, file.Name())
	if err != nil || !match {
		return errDec
	}
	if !strings.HasSuffix(file.Name(), ".json") {
		return errDec
	}
	// #nosec
	fp, err := os.Open(path.Join(filepath, file.Name()))
	if err != nil {
		klog.Infof("node-cache: open file: %s err %v", file.Name(), err)
		return errDec
	}
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(destObj); err != nil {
		if err = fp.Close(); err != nil {
			return errors.Wrapf(err, "failed to close file %s", file.Name())

		}
		return errors.Wrapf(err, "node-cache: couldn't decode file %s", file.Name())
	}
	return nil

}

// Create creates the metadata file in cache directory with identifier name
func (nc *NodeCache) Create(identifier string, data interface{}) error {
	file := path.Join(nc.BasePath, cacheDir, identifier+".json")
	fp, err := os.Create(file)
	if err != nil {
		return errors.Wrapf(err, "node-cache: failed to create metadata storage file %s\n", file)
	}

	defer func() {
		if err = fp.Close(); err != nil {
			klog.Warningf("failed to close file:%s %v", fp.Name(), err)
		}
	}()

	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(data); err != nil {
		return errors.Wrapf(err, "node-cache: failed to encode metadata for file: %s\n", file)
	}
	klog.V(4).Infof("node-cache: successfully saved metadata into file: %s\n", file)
	return nil
}

// Get retrieves the metadata from cache directory with identifier name
func (nc *NodeCache) Get(identifier string, data interface{}) error {
	file := path.Join(nc.BasePath, cacheDir, identifier+".json")
	// #nosec
	fp, err := os.Open(file)
	if err != nil {
		return errors.Wrapf(err, "node-cache: open error for %s", file)
	}

	defer func() {
		if err = fp.Close(); err != nil {
			klog.Warningf("failed to close file:%s %v", fp.Name(), err)
		}
	}()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(data); err != nil {
		return errors.Wrap(err, "rbd: decode error")
	}

	return nil
}

// Delete deletes the metadata file from cache directory with identifier name
func (nc *NodeCache) Delete(identifier string) error {
	file := path.Join(nc.BasePath, cacheDir, identifier+".json")
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return errors.Wrapf(err, "node-cache: error removing file %s", file)
		}
	}
	klog.V(4).Infof("node-cache: successfully deleted metadata storage file at: %+v\n", file)
	return nil
}
