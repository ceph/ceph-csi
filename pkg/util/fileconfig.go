/*
Copyright 2019 ceph-csi authors.

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
    "fmt"
    "io/ioutil"
    "strings"
)

/* FileConfig processes config information stored in files, mostly mapped into
   the runtime container.

   The calls explicitly do not cache any information, to ensure that updated
   configuration is always read from the files (for example when these are
   mapped in as k8s config maps or secrets).

   The BasePath is the path where config files are found, and config files are
   expected to be named in the following manner,
    - BasePath/ceph-cluster-<cluster-fsid>/cluster-config
    - BasePath/ceph-cluster-<cluster-fsid>-provisioner-secret/credentials
    - BasePath/ceph-cluster-<cluster-fsid>-provisioner-secret/subjectid
    - BasePath/ceph-cluster-<cluster-fsid>-publish-secret/credentials
    - BasePath/ceph-cluster-<cluster-fsid>-publish-secret/subjectid
    Where,
        - cluster-fsid is the Ceph cluster fsid in UUID ascii notation
        - The cluster-fsid corresponds to the cluster for which the
        configuration information is present in the mentioned files
        - cluster-config is expected to be a JSON blob with the following
        structure,
        {
            "version": 1,
            "cluster-config": {
                "cluster-fsid": "<ceph-fsid>",
                "monitors": [
                    "IP/DNS:port",
                    "IP/DNS:port"
                ],
                "pools": [
                    "<pool-name>",
                    "<pool-name>"
                ]
            }
        }
        - credentials is expected to contain Base64 encoded credentials for the
        user encoded in subjectid
        - subjectid is the username/subject to use with calls to Ceph, and is
        also Base64 encoded
        - Provisioner secret contains secrets to use by the provisioning system
        - Publish secret contains secrets to use by the publishing/staging
        system
*/

// FileConfig type with basepath that points to source of all config files
type FileConfig struct {
    BasePath string
}

// ClusterConfigv1 strongly typed JSON spec for cluster-config above
type ClusterConfigv1 struct {
    ClusterFsID string   `json:"cluster-fsid"`
    Monitors    []string `json:"monitors"`
    Pools       []string `json:"pools"`
}

// ClusterConfigJSONv1 strongly typed JSON spec for cluster-config above
type ClusterConfigJSONv1 struct {
    Version     int              `json:"version"`
    ClusterConf *ClusterConfigv1 `json:"cluster-config"`
}

// Constants and enum for constructPath operation
type pathType int

const (
    clusterConfig pathType = 0
    pubSubject    pathType = 1
    pubCreds      pathType = 2
    provSubject   pathType = 3
    provCreds     pathType = 4
)

const (
    fNamePrefix      = "ceph-cluster"
    fNameSep         = "-"
    fNamePubPrefix   = "publish-secret"
    fNameProvPrefix  = "provisioner-secret"
    fNameCephConfig  = "cluster-config"
    fNamePubSubject  = "subjectid"
    fNameProvSubject = "subjectid"
    fNamePubCred     = "credentials"
    fNameProvCred    = "credentials"
)

// constructPath constructs well defined paths based on the type of config
// file that needs to be accessed.
func (pType pathType) constructPath(basepath string, fsid string) (filePath string, noerr error) {
    if fsid == "" || basepath == "" {
        return "", fmt.Errorf("missing/empty fsid (%s) or basepath (%s) for config files", fsid, basepath)
    }

    switch pType {
    case clusterConfig:
        filePath = basepath + "/" + fNamePrefix + fNameSep + fsid +
            "/" + fNameCephConfig
    case pubSubject:
        filePath = basepath + "/" + fNamePrefix + fNameSep + fsid +
            fNameSep + fNamePubPrefix + "/" + fNamePubSubject
    case pubCreds:
        filePath = basepath + "/" + fNamePrefix + fNameSep + fsid +
            fNameSep + fNamePubPrefix + "/" + fNamePubCred
    case provSubject:
        filePath = basepath + "/" + fNamePrefix + fNameSep + fsid +
            fNameSep + fNameProvPrefix + "/" + fNameProvSubject
    case provCreds:
        filePath = basepath + "/" + fNamePrefix + fNameSep + fsid +
            fNameSep + fNameProvPrefix + "/" + fNameProvCred
    default:
        return "", fmt.Errorf("invalid path type (%d) specified", pType)
    }

    return
}

// GetMons returns a comma separated MON list, that is read in from the config
// files, based on the passed in fsid
func (fc *FileConfig) GetMons(fsid string) (string, error) {
    fPath, err := clusterConfig.constructPath(fc.BasePath, fsid)
    if err != nil {
        return "", err
    }

    // #nosec
    contentRaw, err := ioutil.ReadFile(fPath)
    if err != nil {
        return "", err
    }

    var cephConfig ClusterConfigJSONv1

    err = json.Unmarshal(contentRaw, &cephConfig)
    if err != nil {
        return "", err
    }

    if cephConfig.ClusterConf.ClusterFsID != fsid {
        return "", fmt.Errorf("mismatching Ceph cluster fsid (%s) in file, passed in (%s)", cephConfig.ClusterConf.ClusterFsID, fsid)
    }

    if len(cephConfig.ClusterConf.Monitors) == 0 {
        return "", fmt.Errorf("monitor list empty in configuration file")
    }

    return strings.Join(cephConfig.ClusterConf.Monitors, ","), nil
}

// GetProvisionerSubjectID returns the provisioner subject ID from the on-disk
// configuration file, based on the passed in fsid
func (fc *FileConfig) GetProvisionerSubjectID(fsid string) (string, error) {
    fPath, err := provSubject.constructPath(fc.BasePath, fsid)
    if err != nil {
        return "", err
    }

    // #nosec
    contentRaw, err := ioutil.ReadFile(fPath)
    if err != nil {
        return "", err
    }

    if string(contentRaw) == "" {
        return "", fmt.Errorf("missing/empty provisioner subject ID from file (%s)", fPath)
    }

    return string(contentRaw), nil
}

// GetPublishSubjectID returns the publish subject ID from the on-disk
// configuration file, based on the passed in fsid
func (fc *FileConfig) GetPublishSubjectID(fsid string) (string, error) {
    fPath, err := pubSubject.constructPath(fc.BasePath, fsid)
    if err != nil {
        return "", err
    }

    // #nosec
    contentRaw, err := ioutil.ReadFile(fPath)
    if err != nil {
        return "", err
    }

    if string(contentRaw) == "" {
        return "", fmt.Errorf("missing/empty publish subject ID from file (%s)", fPath)
    }

    return string(contentRaw), nil
}

// GetCredentialForSubject returns the credentials for the requested subject
// from the cluster config for the passed in fsid
func (fc *FileConfig) GetCredentialForSubject(fsid, subject string) (string, error) {
    var fPath string
    var err error

    tmpSubject, err := fc.GetPublishSubjectID(fsid)
    if err != nil {
        return "", err
    }

    if tmpSubject != subject {
        tmpSubject, err = fc.GetProvisionerSubjectID(fsid)
        if err != nil {
            return "", err
        }

        if tmpSubject != subject {
            return "", fmt.Errorf("requested subject did not match stored publish/provisioner subjectID")
        }

        fPath, err = provCreds.constructPath(fc.BasePath, fsid)
        if err != nil {
            return "", err
        }
    } else {
        fPath, err = pubCreds.constructPath(fc.BasePath, fsid)
        if err != nil {
            return "", err
        }
    }

    // #nosec
    contentRaw, err := ioutil.ReadFile(fPath)
    if err != nil {
        return "", err
    }

    if string(contentRaw) == "" {
        return "", fmt.Errorf("missing/empty credentials in file (%s)", fPath)
    }

    return string(contentRaw), nil
}
