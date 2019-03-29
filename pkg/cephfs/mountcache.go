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

type volumeMountCacheEntry struct {
	DriverName    string `json:"driverName"`
	DriverVersion string `json:"driverVersion"`

	VolumeID      string            `json:"volumeID"`
	Secrets       map[string]string `json:"secrets"`
	StagingPath   string            `json:"stagingPath"`
	TargetPaths   map[string]bool   `json:"targetPaths"`
	CreateTime    time.Time         `json:"createTime"`
	LastMountTime time.Time         `json:"lastMountTime"`
}

type volumeMountCacheMap struct {
	driverName     string
	volumes        map[string]volumeMountCacheEntry
	nodeCacheStore util.NodeCache
	metadataStore  util.CachePersister
}

var (
	volumeMountCachePrefix = "cephfs-mount-cache-"
	volumeMountCache       volumeMountCacheMap
	volumeMountCacheMtx    sync.Mutex
)

func initVolumeMountCache(driverName string, mountCacheDir string, cachePersister util.CachePersister) {
	volumeMountCache.volumes = make(map[string]volumeMountCacheEntry)

	volumeMountCache.driverName = driverName
	volumeMountCache.metadataStore = cachePersister
	volumeMountCache.nodeCacheStore.BasePath = mountCacheDir
	volumeMountCache.nodeCacheStore.CacheDir = ""
	klog.Infof("mount-cache: name: %s, version: %s, mountCacheDir: %s", driverName, version, mountCacheDir)
}

func remountCachedVolumes() error {
	if err := os.MkdirAll(volumeMountCache.nodeCacheStore.BasePath, 0755); err != nil {
		klog.Errorf("mount-cache: failed to create %s: %v", volumeMountCache.nodeCacheStore.BasePath, err)
		return err
	}
	var remountFailCount, remountSuccCount int64
	me := &volumeMountCacheEntry{}
	ce := &controllerCacheEntry{}
	err := volumeMountCache.nodeCacheStore.ForAll(volumeMountCachePrefix, me, func(identifier string) error {
		volID := me.VolumeID
		if err := volumeMountCache.metadataStore.Get(volID, ce); err != nil {
			if err, ok := err.(*util.CacheEntryNotFound); ok {
				klog.Infof("mount-cache: metadata for volume %s not found, assuming the volume to be already deleted (%v)", volID, err)
				if err := volumeMountCache.nodeCacheStore.Delete(genVolumeMountCacheFileName(volID)); err == nil {
					klog.Infof("mount-cache: metadata nofound, delete volume cache entry for volume %s", volID)
				}
			}
		} else {
			if err := mountOneCacheEntry(ce, me); err == nil {
				remountSuccCount++
				volumeMountCache.volumes[me.VolumeID] = *me
				klog.Infof("mount-cache: remount volume %s succ", volID)
			} else {
				remountFailCount++
				klog.Infof("mount-cache: remount volume cache %s fail", volID)
			}
		}
		return nil
	})
	if err != nil {
		klog.Infof("mount-cache: metastore list cache fail %v", err)
		return err
	}
	if remountFailCount > 0 {
		klog.Infof("mount-cache: succ remount %d volumes, fail remount %d volumes", remountSuccCount, remountFailCount)
	} else {
		klog.Infof("mount-cache: volume cache num %d, all succ remount", remountSuccCount)
	}
	return nil
}

func mountOneCacheEntry(ce *controllerCacheEntry, me *volumeMountCacheEntry) error {
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	var (
		err error
		cr  *credentials
	)
	volID := ce.VolumeID
	volOptions := ce.VolOptions

	if volOptions.ProvisionVolume {
		volOptions.RootPath = getVolumeRootPathCeph(volID)
		cr, err = getAdminCredentials(decodeCredentials(me.Secrets))
		if err != nil {
			return err
		}
		var entity *cephEntity
		entity, err = getCephUser(&volOptions, cr, volID)
		if err != nil {
			return err
		}
		cr = entity.toCredentials()
	} else {
		cr, err = getUserCredentials(decodeCredentials(me.Secrets))
		if err != nil {
			return err
		}
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
		if isCorruptedMnt(err) {
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

func isCorruptedMnt(err error) bool {
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
func (mc *volumeMountCacheMap) isEnable() bool {
	//if mount cache dir unset, disable state
	return mc.nodeCacheStore.BasePath != ""
}

func (mc *volumeMountCacheMap) nodeStageVolume(volID string, stagingTargetPath string, secrets map[string]string) error {
	if !mc.isEnable() {
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	lastTargetPaths := make(map[string]bool)
	me, ok := volumeMountCache.volumes[volID]
	if ok {
		if me.StagingPath == stagingTargetPath {
			klog.Warningf("mount-cache: node unexpected restage volume for volume %s", volID)
			return nil
		}
		lastTargetPaths = me.TargetPaths
		klog.Warningf("mount-cache: node stage volume ignore last cache entry for volume %s", volID)
	}

	me = volumeMountCacheEntry{DriverName: mc.driverName, DriverVersion: version}

	me.VolumeID = volID
	me.Secrets = encodeCredentials(secrets)
	me.StagingPath = stagingTargetPath
	me.TargetPaths = lastTargetPaths

	me.CreateTime = time.Now()
	volumeMountCache.volumes[volID] = me
	if err := mc.nodeCacheStore.Create(genVolumeMountCacheFileName(volID), me); err != nil {
		return err
	}
	return nil
}

func (mc *volumeMountCacheMap) nodeUnStageVolume(volID string) error {
	if !mc.isEnable() {
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()
	delete(volumeMountCache.volumes, volID)
	if err := mc.nodeCacheStore.Delete(genVolumeMountCacheFileName(volID)); err != nil {
		return err
	}
	return nil
}

func (mc *volumeMountCacheMap) nodePublishVolume(volID string, targetPath string, readOnly bool) error {
	if !mc.isEnable() {
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	_, ok := volumeMountCache.volumes[volID]
	if !ok {
		return errors.New("mount-cache: node publish volume failed to find cache entry for volume")
	}
	volumeMountCache.volumes[volID].TargetPaths[targetPath] = readOnly
	return mc.updateNodeCache(volID)
}

func (mc *volumeMountCacheMap) nodeUnPublishVolume(volID string, targetPath string) error {
	if !mc.isEnable() {
		return nil
	}
	volumeMountCacheMtx.Lock()
	defer volumeMountCacheMtx.Unlock()

	_, ok := volumeMountCache.volumes[volID]
	if !ok {
		return errors.New("mount-cache: node unpublish volume failed to find cache entry for volume")
	}
	delete(volumeMountCache.volumes[volID].TargetPaths, targetPath)
	return mc.updateNodeCache(volID)
}

func (mc *volumeMountCacheMap) updateNodeCache(volID string) error {
	me := volumeMountCache.volumes[volID]
	if err := volumeMountCache.nodeCacheStore.Delete(genVolumeMountCacheFileName(volID)); err == nil {
		klog.Infof("mount-cache: metadata nofound, delete mount cache failed for volume %s", volID)
	}
	if err := mc.nodeCacheStore.Create(genVolumeMountCacheFileName(volID), me); err != nil {
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
