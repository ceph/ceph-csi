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

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type NodeCache struct {
	BasePath string
}

var cacheDir = "controller"

func (nc *NodeCache) EnsureCacheDirectory(cacheDir string) error {
	fullPath := path.Join(nc.BasePath, cacheDir)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		if err := os.Mkdir(fullPath, 0755); err != nil {
			return errors.Wrapf(err, "node-cache: failed to create %s folder with error: %v", fullPath, err)
		}
	}
	return nil
}

func (nc *NodeCache) ForAll(pattern string, destObj interface{}, f ForAllFunc) error {
	err := nc.EnsureCacheDirectory(cacheDir)
	if err != nil {
		return errors.Wrap(err, "node-cache: couldn't ensure cache directory exists")
	}
	files, err := ioutil.ReadDir(path.Join(nc.BasePath, cacheDir))
	if err != nil {
		return errors.Wrapf(err, "node-cache: failed to read %s folder", nc.BasePath)
	}

	for _, file := range files {
		match, err := regexp.MatchString(pattern, file.Name())
		if err != nil || !match {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		fp, err := os.Open(path.Join(nc.BasePath, cacheDir, file.Name()))
		if err != nil {
			glog.Infof("node-cache: open file: %s err %v", file.Name(), err)
			continue
		}
		decoder := json.NewDecoder(fp)
		if err = decoder.Decode(destObj); err != nil {
			fp.Close()
			return errors.Wrapf(err, "node-cache: couldn't decode file %s", file.Name())
		}
		if err := f(strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))); err != nil {
			return err
		}
	}
	return nil
}

func (nc *NodeCache) Create(identifier string, data interface{}) error {
	file := path.Join(nc.BasePath, cacheDir, identifier+".json")
	fp, err := os.Create(file)
	if err != nil {
		return errors.Wrapf(err, "node-cache: failed to create metadata storage file %s\n", file)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(data); err != nil {
		return errors.Wrapf(err, "node-cache: failed to encode metadata for file: %s\n", file)
	}
	glog.V(4).Infof("node-cache: successfully saved metadata into file: %s\n", file)
	return nil
}

func (nc *NodeCache) Get(identifier string, data interface{}) error {
	file := path.Join(nc.BasePath, cacheDir, identifier+".json")
	fp, err := os.Open(file)
	if err != nil {
		return errors.Wrapf(err, "node-cache: open error for %s", file)
	}
	defer fp.Close()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(data); err != nil {
		return errors.Wrap(err, "rbd: decode error")
	}

	return nil
}

func (nc *NodeCache) Delete(identifier string) error {
	file := path.Join(nc.BasePath, cacheDir, identifier+".json")
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return errors.Wrapf(err, "node-cache: error removing file %s", file)
		}
	}
	glog.V(4).Infof("node-cache: successfully deleted metadata storage file at: %+v\n", file)
	return nil
}
