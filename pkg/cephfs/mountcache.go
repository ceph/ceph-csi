package cephfs

import (
	"encoding/base64"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/ceph/ceph-csi/pkg/util"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

type volumeMountEntry struct {
	NodeID        string `json:"nodeID"`
	DriverName    string `json:"driverName"`
	DriverVersion string `json:"driverVersion"`

	Namespace string `json:"namespace"`

	VolumeID      string            `json:"volumeID"`
	Secrets       map[string]string `json:"secrets"`
	StagingPath   string            `json:"stagingPath"`
	TargetPaths   map[string]bool   `json:"targetPaths"`
	CreateTime    time.Time         `json:"createTime"`
	LastMountTime time.Time         `json:"lastMountTime"`
	LoadCount     uint64            `json:"loadCount"`
}

type volumeMountCacheMap struct {
	DriverName     string
	DriverVersion  string
	NodeID         string
	MountFailNum   int64
	MountSuccNum   int64
	Volumes        map[string]volumeMountEntry
	NodeCacheStore util.NodeCache
	MetadataStore  util.CachePersister
}

var (
	MountCacheDir          = ""
	volumeMountCachePrefix = "cephfs-mount-cache-"
	volumeMountCache       volumeMountCacheMap
	volumeMountCacheMtx    sync.Mutex
)

func remountHisMountedPath(name string, v string, nodeID string, cachePersister util.CachePersister) error {
	volumeMountCache.Volumes = make(map[string]volumeMountEntry)
	volumeMountCache.NodeID = nodeID
	volumeMountCache.DriverName = name
	volumeMountCache.DriverVersion = v
	volumeMountCache.MountSuccNum = 0
	volumeMountCache.MountFailNum = 0

	volumeMountCache.MetadataStore = cachePersister

	volumeMountCache.NodeCacheStore.BasePath = MountCacheDir
	volumeMountCache.NodeCacheStore.CacheDir = ""

	if len(MountCacheDir) == 0 {
		//if mount cache dir unset, disable remount
		klog.Infof("mount-cache: mountcachedir no define disalbe mount cache.")
		return nil
	}

	klog.Infof("mount-cache: MountCacheDir: %s", MountCacheDir)
	if err := os.MkdirAll(volumeMountCache.NodeCacheStore.BasePath, 0755); err != nil {
		klog.Errorf("mount-cache: failed to create %s: %v", volumeMountCache.NodeCacheStore.BasePath, err)
		return err
	}
	me := &volumeMountEntry{}
	ce := &controllerCacheEntry{}
	err := volumeMountCache.NodeCacheStore.ForAll(volumeMountCachePrefix, me, func(identifier string) error {
		volID := me.VolumeID
		klog.Infof("mount-cache: load %v", me)
		if err := volumeMountCache.MetadataStore.Get(volID, ce); err != nil {
			if err, ok := err.(*util.CacheEntryNotFound); ok {
				klog.Infof("cephfs: metadata for volume %s not found, assuming the volume to be already deleted (%v)", volID, err)
				if err := volumeMountCache.NodeCacheStore.Delete(genVolumeMountCacheFileName(volID)); err == nil {
					klog.Infof("mount-cache: metadata nofound, delete volume cache entry for volume %s", volID)
				}
			}
		} else {
			if err := mountOneCacheEntry(ce, me); err == nil {
				volumeMountCache.MountSuccNum++
				volumeMountCache.Volumes[me.VolumeID] = *me
			} else {
				volumeMountCache.MountFailNum++
			}
		}
		return nil
	})
	if err != nil {
		klog.Infof("mount-cache: metastore list cache fail %v", err)
		return err
	}
	if volumeMountCache.MountFailNum > volumeMountCache.MountSuccNum {
		return errors.New("mount-cache: too many volumes mount fail")
	}
	klog.Infof("mount-cache: succ remount %d volumes, fail remount %d volumes", volumeMountCache.MountSuccNum, volumeMountCache.MountFailNum)
	return nil
}

func mountOneCacheEntry(ce *controllerCacheEntry, me *volumeMountEntry) error {
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	var err error
	volID := ce.VolumeID
	volOptions := ce.VolOptions

	adminCr, err := getAdminCredentials(decodeCredentials(me.Secrets))
	if err != nil {
		return err
	}
	entity, err := getCephUser(&volOptions, adminCr, volID)
	if err != nil {
		klog.Infof("mount-cache: failed to get ceph user: %s %v", volID, me.StagingPath)
	}
	cr := entity.toCredentials()

	if volOptions.ProvisionVolume {
		volOptions.RootPath = getVolumeRootPathCeph(volID)
	}

	err = cleanupMountPoint(me.StagingPath)
	if err != nil {
		klog.Infof("mount-cache: failed to cleanup volume mount point %s, remove it: %s %v", volID, me.StagingPath, err)
		return err
	}

	isMnt, err := isMountPoint(me.StagingPath)
	if err != nil {
		isMnt = false
		klog.Infof("mount-cache: failed to check volume mounted %s: %s %v", volID, me.StagingPath, err)
	}

	if !isMnt {
		m, err := newMounter(&volOptions)
		if err != nil {
			klog.Errorf("mount-cache: failed to create mounter for volume %s: %v", volID, err)
			return err
		}
		if err := m.mount(me.StagingPath, cr, &volOptions); err != nil {
			klog.Errorf("mount-cache: failed to mount volume %s: %v", volID, err)
			return err
		}
	}
	for targetPath, readOnly := range me.TargetPaths {
		if err := cleanupMountPoint(targetPath); err == nil {
			if err := bindMount(me.StagingPath, targetPath, readOnly); err != nil {
				klog.Errorf("mount-cache: failed to bind-mount volume %s: %s %s %v %v",
					volID, me.StagingPath, targetPath, readOnly, err)
			} else {
				klog.Infof("mount-cache: succ bind-mount volume %s: %s %s %v",
					volID, me.StagingPath, targetPath, readOnly)
			}
		}
	}
	return nil
}

func cleanupMountPoint(mountPoint string) error {
	if _, err := os.Stat(mountPoint); err != nil {
		if IsCorruptedMnt(err) {
			klog.Infof("mount-cache: corrupted mount point %s, need unmount", mountPoint)
			err := execCommandErr("umount", mountPoint)
			if err != nil {
				klog.Infof("mount-cache: unmount %s fail %v", mountPoint, err)
				//ignore error return err
			}
		}
	}
	if _, err := os.Stat(mountPoint); err != nil {
		klog.Errorf("mount-cache: mount point %s stat fail %v", mountPoint, err)
		return err
	}
	return nil
}

func IsCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pe := err.(type) {
	case nil:
		return false
	case *os.PathError:
		underlyingError = pe.Err
	case *os.LinkError:
		underlyingError = pe.Err
	case *os.SyscallError:
		underlyingError = pe.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO || underlyingError == syscall.EACCES
}

func genVolumeMountCacheFileName(volID string) string {
	cachePath := volumeMountCachePrefix + volID
	return cachePath
}

func (mc *volumeMountCacheMap) nodeStageVolume(volID string, stagingTargetPath string, secrets map[string]string) error {
	if len(MountCacheDir) == 0 {
		//if mount cache dir unset, disable remount
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	lastTargetPaths := make(map[string]bool)
	me, ok := volumeMountCache.Volumes[volID]
	if ok {
		if me.StagingPath == stagingTargetPath {
			klog.Warningf("mount-cache: node unexpected restage volume for volume %s", volID)
			return nil
		}
		lastTargetPaths = me.TargetPaths
		klog.Warningf("mount-cache: node stage volume ignore last cache entry for volume %s", volID)
	}

	me = volumeMountEntry{NodeID: mc.NodeID, DriverName: mc.DriverName, DriverVersion: mc.DriverVersion}

	me.VolumeID = volID
	me.Secrets = encodeCredentials(secrets)
	me.StagingPath = stagingTargetPath
	me.TargetPaths = lastTargetPaths

	curTime := time.Now()
	me.CreateTime = curTime
	me.CreateTime = curTime
	me.LoadCount = 0
	volumeMountCache.Volumes[volID] = me
	if err := mc.NodeCacheStore.Create(genVolumeMountCacheFileName(volID), me); err != nil {
		klog.Errorf("mount-cache: node stage volume failed to store a cache entry for volume %s: %v", volID, err)
		return err
	}
	klog.Infof("mount-cache: node stage volume succ to store a cache entry for volume %s: %v", volID, me)
	return nil
}

func (mc *volumeMountCacheMap) nodeUnStageVolume(volID string, stagingTargetPath string) error {
	if len(MountCacheDir) == 0 {
		//if mount cache dir unset, disable remount
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()
	delete(volumeMountCache.Volumes, volID)
	if err := mc.NodeCacheStore.Delete(genVolumeMountCacheFileName(volID)); err != nil {
		klog.Infof("mount-cache: node unstage volume failed to delete cache entry for volume %s: %s %v", volID, stagingTargetPath, err)
		return err
	}
	return nil
}

func (mc *volumeMountCacheMap) nodePublishVolume(volID string, targetPath string, readOnly bool) error {
	if len(MountCacheDir) == 0 {
		//if mount cache dir unset, disable remount
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	_, ok := volumeMountCache.Volumes[volID]
	if !ok {
		klog.Errorf("mount-cache: node publish volume failed to find cache entry for volume %s", volID)
		return errors.New("mount-cache: node publish volume failed to find cache entry for volume")
	}
	volumeMountCache.Volumes[volID].TargetPaths[targetPath] = readOnly
	return mc.updateNodeCache(volID)
}

func (mc *volumeMountCacheMap) nodeUnPublishVolume(volID string, targetPath string) error {
	if len(MountCacheDir) == 0 {
		//if mount cache dir unset, disable remount
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	_, ok := volumeMountCache.Volumes[volID]
	if !ok {
		klog.Errorf("mount-cache: node unpublish volume failed to find cache entry for volume %s", volID)
		return errors.New("mount-cache: node unpublish volume failed to find cache entry for volume")
	}
	delete(volumeMountCache.Volumes[volID].TargetPaths, targetPath)
	return mc.updateNodeCache(volID)
}

func (mc *volumeMountCacheMap) updateNodeCache(volID string) error {
	me := volumeMountCache.Volumes[volID]
	if err := volumeMountCache.NodeCacheStore.Delete(genVolumeMountCacheFileName(volID)); err == nil {
		klog.Infof("mount-cache: metadata nofound, delete mount cache failed for volume %s", volID)
	}
	if err := mc.NodeCacheStore.Create(genVolumeMountCacheFileName(volID), me); err != nil {
		klog.Errorf("mount-cache: mount cache failed to update for volume %s: %v", volID, err)
		return err
	}
	return nil
}

func encodeCredentials(input map[string]string) (output map[string]string) {
	output = make(map[string]string)
	for key, value := range input {
		nKey := base64.StdEncoding.EncodeToString([]byte(key))
		nValue := base64.StdEncoding.EncodeToString([]byte(value))
		output[nKey] = nValue
	}
	return output
}

func decodeCredentials(input map[string]string) (output map[string]string) {
	output = make(map[string]string)
	for key, value := range input {
		nKey, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			klog.Errorf("mount-cache: decode secret fail")
			continue
		}
		nValue, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			klog.Errorf("mount-cache: decode secret fail")
			continue
		}
		output[string(nKey)] = string(nValue)
	}
	return output
}
