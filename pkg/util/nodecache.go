package util

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type NodeCache struct {
	BasePath string
}

var (
	CacheSubPath string

	nodeCacheMtx sync.Mutex
)

func (nc *NodeCache) ForAll(pattern string, destObj interface{}, f ForAllFunc) error {
	files, err := ioutil.ReadDir(path.Join(nc.BasePath, CacheSubPath))
	if err != nil {
		glog.Infof("node-cache: failed to read %s folder", nc.BasePath)
		return errors.Wrap(err, "node-cache: list files error")
	}

	nodeCacheMtx.Lock()
	defer nodeCacheMtx.Unlock()

	for _, file := range files {
		match, err := regexp.MatchString(pattern, file.Name())
		if err != nil || !match {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		fp, err := os.Open(path.Join(nc.BasePath, CacheSubPath, file.Name()))
		if err != nil {
			glog.Infof("node-cache: open file: %s err %%v", file.Name(), err)
			continue
		}
		decoder := json.NewDecoder(fp)
		if err = decoder.Decode(destObj); err != nil {
                        glog.Infof("node-cache: decode file: %s err: %v", file.Name(), err)
                        fp.Close()
                        continue
		}
		if err := f(strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))); err != nil {
			return err
		}
	}
	return nil
}

func (nc *NodeCache) Create(identifier string, data interface{}) error {
	nodeCacheMtx.Lock()
	defer nodeCacheMtx.Unlock()

	file := path.Join(nc.BasePath, CacheSubPath, identifier+".json")
	fp, err := os.Create(file)
	if err != nil {
		glog.Errorf("node-cache: failed to create metadata storage file %s with error: %v\n", file, err)
		return errors.Wrapf(err, "rbd: create error for %s", file)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(data); err != nil {
		glog.Errorf("node-cache: failed to encode metadata for file: %s with error: %v\n", file, err)
		return errors.Wrap(err, "node-cache: encode error")
	}
	glog.Infof("node-cache: successfully saved metadata into file: %s\n", file)
	return nil
}

func (nc *NodeCache) Get(identifier string, data interface{}) error {
	nodeCacheMtx.Lock()
	defer nodeCacheMtx.Unlock()

	file := path.Join(nc.BasePath, CacheSubPath, identifier+".json")
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
	nodeCacheMtx.Lock()
	defer nodeCacheMtx.Unlock()

        file := path.Join(nc.BasePath, CacheSubPath, identifier+".json")
        glog.Infof("node-cache: deleting metadata storage file at: %+v\n", file)
        err := os.Remove(file)
        if err != nil {
                if err != os.ErrNotExist {
                        return errors.Wrapf(err, "node-cache: error removing file %s", file)
                }
        }
        return nil
}
