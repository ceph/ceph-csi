/*
Copyright 2021 The Ceph-CSI Authors.

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

package e2e

import (
	"context"
	"crypto/md5" //nolint:gosec // hash generation
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	appsv1 "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubectl "k8s.io/kubernetes/test/e2e/framework/kubectl"
)

/* #nosec:G101, values not credentials, just a reference to the location.*/
const (
	defaultNs     = "default"
	defaultSCName = ""

	rbdType    = "rbd"
	cephfsType = "cephfs"

	volumesType = "volumes"
	snapsType   = "snaps"

	rookToolBoxPodLabel = "app=rook-ceph-tools"
	rbdMountOptions     = "mountOptions"

	retainPolicy = v1.PersistentVolumeReclaimRetain
	// deletePolicy is the default policy in E2E.
	deletePolicy = v1.PersistentVolumeReclaimDelete
	// Default key and label for Listoptions.
	appKey        = "app"
	appLabel      = "write-data-in-pod"
	appCloneLabel = "app-clone"

	noError = ""
	// labels/selector used to list/delete rbd pods.
	rbdPodLabels = "app in (ceph-csi-rbd, csi-rbdplugin, csi-rbdplugin-provisioner)"

	exitOneErr = "command terminated with exit code 1"

	// cluster Name, set by user.
	clusterNameKey     = "csi.ceph.com/cluster/name"
	defaultClusterName = "k8s-cluster-1"
)

var (
	// cli flags.
	deployTimeout     int
	deployCephFS      bool
	deployRBD         bool
	deployNFS         bool
	testCephFS        bool
	testCephFSFscrypt bool
	testRBD           bool
	testRBDFSCrypt    bool
	testNBD           bool
	testNFS           bool
	helmTest          bool
	upgradeTesting    bool
	upgradeVersion    string
	cephCSINamespace  string
	rookNamespace     string
	radosNamespace    string
	poll              = 2 * time.Second
	isOpenShift       bool
	clusterID         string
	nfsDriverName     string
)

type cephfsFilesystem struct {
	Name         string `json:"name"`
	MetadataPool string `json:"metadata_pool"`
}

// listCephFSFileSystems list CephFS filesystems in json format.
func listCephFSFileSystems(f *framework.Framework) ([]cephfsFilesystem, error) {
	var fsList []cephfsFilesystem

	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		"ceph fs ls --format=json",
		rookNamespace)
	if err != nil {
		return fsList, err
	}
	if stdErr != "" {
		return fsList, fmt.Errorf("error listing fs %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &fsList)
	if err != nil {
		return fsList, err
	}

	return fsList, nil
}

// getCephFSMetadataPoolName get CephFS pool name from filesystem name.
func getCephFSMetadataPoolName(f *framework.Framework, filesystem string) (string, error) {
	fsList, err := listCephFSFileSystems(f)
	if err != nil {
		return "", fmt.Errorf("list CephFS filesystem failed: %w", err)
	}
	for _, v := range fsList {
		if v.Name != filesystem {
			continue
		}

		return v.MetadataPool, nil
	}

	return "", fmt.Errorf("metadata pool name not found for filesystem: %s", filesystem)
}

func compareStdoutWithCount(stdOut string, count int) error {
	stdOut = strings.TrimSuffix(stdOut, "\n")
	res, err := strconv.Atoi(stdOut)
	if err != nil {
		return fmt.Errorf("failed to convert string to int: %v", stdOut)
	}
	if res != count {
		return fmt.Errorf("expected omap object count %d, got %d", count, res)
	}

	return nil
}

// validateOmapCount validates no of OMAP entries on the given pool.
// Works with Cephfs and RBD drivers and mode can be snapsType or volumesType.
func validateOmapCount(f *framework.Framework, count int, driver, pool, mode string) {
	type radosListCommand struct {
		volumeMode                           string
		driverType                           string
		radosLsCmd, radosLsCmdFilter         string
		radosLsKeysCmd, radosLsKeysCmdFilter string
	}

	radosListCommands := []radosListCommand{
		{
			volumeMode: volumesType,
			driverType: cephfsType,
			radosLsCmd: fmt.Sprintf("rados ls --pool=%s --namespace csi", pool),
			radosLsCmdFilter: fmt.Sprintf("rados ls --pool=%s --namespace csi | grep -v default | grep -c ^csi.volume.",
				pool),
			radosLsKeysCmd: fmt.Sprintf("rados listomapkeys csi.volumes.default --pool=%s --namespace csi", pool),
			radosLsKeysCmdFilter: fmt.Sprintf("rados listomapkeys csi.volumes.default --pool=%s --namespace csi|wc -l",
				pool),
		},
		{
			volumeMode:           volumesType,
			driverType:           rbdType,
			radosLsCmd:           fmt.Sprintf("rados ls %s", rbdOptions(pool)),
			radosLsCmdFilter:     fmt.Sprintf("rados ls %s | grep -v default | grep -c ^csi.volume.", rbdOptions(pool)),
			radosLsKeysCmd:       fmt.Sprintf("rados listomapkeys csi.volumes.default %s", rbdOptions(pool)),
			radosLsKeysCmdFilter: fmt.Sprintf("rados listomapkeys csi.volumes.default %s | wc -l", rbdOptions(pool)),
		},
		{
			volumeMode: snapsType,
			driverType: cephfsType,
			radosLsCmd: fmt.Sprintf("rados ls --pool=%s --namespace csi", pool),
			radosLsCmdFilter: fmt.Sprintf("rados ls --pool=%s --namespace csi | grep -v default | grep -c ^csi.snap.",
				pool),
			radosLsKeysCmd: fmt.Sprintf("rados listomapkeys csi.snaps.default --pool=%s --namespace csi", pool),
			radosLsKeysCmdFilter: fmt.Sprintf("rados listomapkeys csi.snaps.default --pool=%s --namespace csi|wc -l",
				pool),
		},
		{
			volumeMode:           snapsType,
			driverType:           rbdType,
			radosLsCmd:           fmt.Sprintf("rados ls %s", rbdOptions(pool)),
			radosLsCmdFilter:     fmt.Sprintf("rados ls %s | grep -v default | grep -c ^csi.snap.", rbdOptions(pool)),
			radosLsKeysCmd:       fmt.Sprintf("rados listomapkeys csi.snaps.default %s", rbdOptions(pool)),
			radosLsKeysCmdFilter: fmt.Sprintf("rados listomapkeys csi.snaps.default %s | wc -l", rbdOptions(pool)),
		},
	}

	for _, cmds := range radosListCommands {
		if !strings.EqualFold(cmds.volumeMode, mode) || !strings.EqualFold(cmds.driverType, driver) {
			continue
		}
		filterCmds := []string{cmds.radosLsCmdFilter, cmds.radosLsKeysCmdFilter}
		filterLessCmds := []string{cmds.radosLsCmd, cmds.radosLsKeysCmd}
		for i, cmd := range filterCmds {
			stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
			if err != nil {
				if !strings.Contains(err.Error(), exitOneErr) {
					framework.Failf("failed to execute rados command '%s' : err=%v stdErr=%s", cmd, err, stdErr)
				}
			}
			if stdErr != "" {
				framework.Failf("failed to execute rados command '%s' : stdErr=%s", cmd, stdErr)
			}
			err = compareStdoutWithCount(stdOut, count)
			if err == nil {
				continue
			}
			saveErr := err
			if strings.Contains(err.Error(), "expected omap object count") {
				stdOut, stdErr, err = execCommandInToolBoxPod(f, filterLessCmds[i], rookNamespace)
				if err == nil {
					framework.Logf("additional debug info: rados ls command output: %s, stdErr: %s", stdOut, stdErr)
				}
			}
			framework.Failf("%v", saveErr)
		}
	}
}

func getMons(ns string, c kubernetes.Interface) ([]string, error) {
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	services := make([]string, 0)

	var svcList *v1.ServiceList
	t := time.Duration(deployTimeout) * time.Minute
	err := wait.PollUntilContextTimeout(context.TODO(), poll, t, true, func(ctx context.Context) (bool, error) {
		var svcErr error
		svcList, svcErr = c.CoreV1().Services(ns).List(ctx, opt)
		if svcErr != nil {
			if isRetryableAPIError(svcErr) {
				return false, nil
			}

			return false, fmt.Errorf("failed to list Services in namespace %q: %w", ns, svcErr)
		}

		return true, nil
	})
	if err != nil {
		return services, fmt.Errorf("could not get Services: %w", err)
	}
	for i := range svcList.Items {
		s := fmt.Sprintf(
			"%s.%s.svc.cluster.local:%d",
			svcList.Items[i].Name,
			svcList.Items[i].Namespace,
			svcList.Items[i].Spec.Ports[0].Port)
		services = append(services, s)
	}

	return services, nil
}

func getMonsHash(mons string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(mons))) //nolint:gosec // hash generation
}

func getClusterID(f *framework.Framework) (string, error) {
	if clusterID != "" {
		return clusterID, nil
	}

	fsID, stdErr, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return "", fmt.Errorf("failed getting clusterID through toolbox: %w", err)
	}
	if stdErr != "" {
		return "", fmt.Errorf("error getting fsid: %s", stdErr)
	}
	// remove new line present in fsID
	return strings.Trim(fsID, "\n"), nil
}

func getStorageClass(path string) (scv1.StorageClass, error) {
	sc := scv1.StorageClass{}
	err := unmarshal(path, &sc)

	return sc, err
}

func getSecret(path string) (v1.Secret, error) {
	sc := v1.Secret{}
	err := unmarshal(path, &sc)
	// discard corruptInputError
	if err != nil {
		var b64cie base64.CorruptInputError
		if !errors.As(err, &b64cie) {
			return sc, err
		}
	}

	return sc, nil
}

func deleteResource(scPath string) error {
	data, err := replaceNamespaceInTemplate(scPath)
	if err != nil {
		framework.Logf("failed to read content from %s %v", scPath, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout, "--ignore-not-found=true")
	if err != nil {
		framework.Logf("failed to delete %s %v", scPath, err)
	}

	return err
}

func unmarshal(fileName string, obj interface{}) error {
	f, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	data, err := utilyaml.ToJSON(f)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, obj)

	return err
}

// createPVCAndApp creates pvc and pod
// if name is not empty same will be set as pvc and app name.
func createPVCAndApp(
	name string,
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	app *v1.Pod,
	pvcTimeout int,
) error {
	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}
	err := createPVCAndvalidatePV(f.ClientSet, pvc, pvcTimeout)
	if err != nil {
		return err
	}
	err = createApp(f.ClientSet, app, deployTimeout)

	return err
}

// createPVCAndDeploymentApp creates pvc and deployment.
func createPVCAndDeploymentApp(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	app *appsv1.Deployment,
	pvcTimeout int,
) error {
	err := createPVCAndvalidatePV(f.ClientSet, pvc, pvcTimeout)
	if err != nil {
		return err
	}
	err = createDeploymentApp(f.ClientSet, app, deployTimeout)

	return err
}

// validatePVCAndDeploymentAppBinding creates PVC and Deployment, and waits until
// all its replicas are Running. Use `replicas` to override default number of replicas
// defined in `deploymentPath` Deployment manifest.
func validatePVCAndDeploymentAppBinding(
	f *framework.Framework,
	pvcPath string,
	deploymentPath string,
	namespace string,
	replicas *int32,
	pvcTimeout int,
) (*v1.PersistentVolumeClaim, *appsv1.Deployment, error) {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load PVC: %w", err)
	}
	pvc.Namespace = namespace

	depl, err := loadAppDeployment(deploymentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load Deployment: %w", err)
	}
	depl.Namespace = f.UniqueName
	if replicas != nil {
		depl.Spec.Replicas = replicas
	}

	err = createPVCAndDeploymentApp(f, pvc, depl, pvcTimeout)
	if err != nil {
		return nil, nil, err
	}

	err = waitForDeploymentComplete(f.ClientSet, depl.Name, depl.Namespace, deployTimeout)
	if err != nil {
		return nil, nil, err
	}

	return pvc, depl, nil
}

// DeletePVCAndDeploymentApp deletes pvc and deployment.
func deletePVCAndDeploymentApp(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	app *appsv1.Deployment,
) error {
	err := deleteDeploymentApp(f.ClientSet, app.Name, app.Namespace, deployTimeout)
	if err != nil {
		return err
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)

	return err
}

// deletePVCAndApp delete pvc and pod
// if name is not empty same will be set as pvc and app name.
func deletePVCAndApp(name string, f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error {
	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}

	err := deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)

	return err
}

func createPVCAndAppBinding(
	pvcPath, appPath string,
	f *framework.Framework,
	pvcTimeout int,
) (*v1.PersistentVolumeClaim, *v1.Pod, error) {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return nil, nil, err
	}
	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return nil, nil, err
	}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, pvcTimeout)
	if err != nil {
		return nil, nil, err
	}

	return pvc, app, nil
}

func validatePVCAndAppBinding(pvcPath, appPath string, f *framework.Framework) error {
	pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	if err != nil {
		return err
	}

	err = deletePVCAndApp("", f, pvc, app)

	return err
}

func getMountType(selector, mountPath string, f *framework.Framework) (string, error) {
	opt := metav1.ListOptions{
		LabelSelector: selector,
	}
	cmd := fmt.Sprintf("lsblk -o TYPE,MOUNTPOINT | grep '%s' | awk '{print $1}'", mountPath)
	stdOut, stdErr, err := execCommandInContainer(f, cmd, cephCSINamespace, "csi-rbdplugin", &opt)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return strings.TrimSpace(stdOut), fmt.Errorf("%s", stdErr)
	}

	return strings.TrimSpace(stdOut), nil
}

func validateNormalUserPVCAccess(pvcPath string, f *framework.Framework) error {
	writeTest := func(ns string, opts *metav1.ListOptions) error {
		_, stdErr, err := execCommandInPod(f, "echo testing > /target/testing", ns, opts)
		if err != nil {
			return fmt.Errorf("failed to exec command in pod: %w", err)
		}
		if stdErr != "" {
			return fmt.Errorf("failed to touch a file as non-root user %v", stdErr)
		}

		return nil
	}

	return validateNormalUserPVCAccessFunc(pvcPath, f, writeTest)
}

func validateInodeCount(pvcPath string, f *framework.Framework, inodes int) error {
	countInodes := func(ns string, opts *metav1.ListOptions) error {
		stdOut, stdErr, err := execCommandInPod(f, "df --output=itotal /target | tail -n1", ns, opts)
		if err != nil {
			return fmt.Errorf("failed to exec command in pod: %w", err)
		}
		if stdErr != "" {
			return fmt.Errorf("failed to list inodes in pod: %v", stdErr)
		}

		itotal, err := strconv.Atoi(strings.TrimSpace(stdOut))
		if err != nil {
			return fmt.Errorf("failed to parse itotal %q to int: %w", strings.TrimSpace(stdOut), err)
		}
		if inodes != itotal {
			return fmt.Errorf("expected inodes (%d) do not match itotal on volume (%d)", inodes, itotal)
		}

		return nil
	}

	return validateNormalUserPVCAccessFunc(pvcPath, f, countInodes)
}

func validateNormalUserPVCAccessFunc(
	pvcPath string,
	f *framework.Framework,
	validate func(ns string, opts *metav1.ListOptions) error,
) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName
	pvc.Name = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}
	var user int64 = 2000
	app := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-run-as-non-root",
			Namespace: f.UniqueName,
			Labels: map[string]string{
				"app": "pod-run-as-non-root",
			},
		},
		Spec: v1.PodSpec{
			SecurityContext: &v1.PodSecurityContext{FSGroup: &user},
			Containers: []v1.Container{
				{
					Name:    "write-pod",
					Image:   "quay.io/centos/centos:latest",
					Command: []string{"/bin/sleep", "999999"},
					SecurityContext: &v1.SecurityContext{
						RunAsUser: &user,
					},
					VolumeMounts: []v1.VolumeMount{
						{
							MountPath: "/target",
							Name:      "target",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "target",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
							ReadOnly:  false,
						},
					},
				},
			},
		},
	}

	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=pod-run-as-non-root",
	}

	err = validate(app.Namespace, &opt)
	if err != nil {
		return fmt.Errorf("failed to run validation function: %w", err)
	}

	// metrics for BlockMode was added in Kubernetes 1.22
	isBlockMode := false
	if pvc.Spec.VolumeMode != nil {
		isBlockMode = (*pvc.Spec.VolumeMode == v1.PersistentVolumeBlock)
	}
	if !isBlockMode && !isOpenShift {
		err = getMetricsForPVC(f, pvc, deployTimeout)
		if err != nil {
			return err
		}
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}

	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)

	return err
}

// writeDataInPod fill zero content to a file in the provided POD volume.
func writeDataInPod(app *v1.Pod, opt *metav1.ListOptions, f *framework.Framework) error {
	app.Namespace = f.UniqueName

	err := createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}

	// write data to PVC. The idea here is to fill some content in the file
	// instead of filling and reverifying the md5sum/data integrity
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
	// While writing more data we are encountering issues in E2E timeout, so keeping it low for now
	_, writeErr, err := execCommandInPod(
		f,
		fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10 status=none", filePath),
		app.Namespace,
		opt)
	if err != nil {
		return err
	}
	if writeErr != "" {
		err = fmt.Errorf("failed to write data %v", writeErr)
	}

	return err
}

func checkDataPersist(pvcPath, appPath string, f *framework.Framework) error {
	data := "checking data persist"
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}

	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-data"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=validate-data",
	}
	// write data to PVC
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

	_, stdErr, err := execCommandInPod(f, fmt.Sprintf("echo %s > %s", data, filePath), app.Namespace, &opt)
	if err != nil {
		return fmt.Errorf("failed to exec command in pod: %w", err)
	}
	if stdErr != "" {
		return fmt.Errorf("failed to write data to a file %v", stdErr)
	}
	// delete app
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}
	// recreate app and check data persist
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	persistData, stdErr, err := execCommandInPod(f, fmt.Sprintf("cat %s", filePath), app.Namespace, &opt)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to get file content %v", stdErr)
	}
	if !strings.Contains(persistData, data) {
		return fmt.Errorf("data not persistent expected data %s received data %s  ", data, persistData)
	}

	err = deletePVCAndApp("", f, pvc, app)

	return err
}

func pvcDeleteWhenPoolNotFound(pvcPath string, cephFS bool, f *framework.Framework) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName

	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}
	if cephFS {
		err = deleteBackingCephFSVolume(f, pvc)
		if err != nil {
			return err
		}
		// delete cephFS filesystem
		err = deletePool(fileSystemName, cephFS, f)
		if err != nil {
			return err
		}
	} else {
		err = deleteBackingRBDImage(f, pvc)
		if err != nil {
			return err
		}
		// delete rbd pool
		err = deletePool(defaultRBDPool, cephFS, f)
		if err != nil {
			return err
		}
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)

	return err
}

func checkMountOptions(pvcPath, appPath string, f *framework.Framework, mountFlags []string) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}

	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-mount-opt"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=validate-mount-opt",
	}

	cmd := fmt.Sprintf("mount |grep %s", app.Spec.Containers[0].VolumeMounts[0].MountPath)
	data, stdErr, err := execCommandInPod(f, cmd, app.Namespace, &opt)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to get mount point %v", stdErr)
	}
	for _, f := range mountFlags {
		if !strings.Contains(data, f) {
			return fmt.Errorf("mount option %s not found in %s", f, data)
		}
	}

	err = deletePVCAndApp("", f, pvc, app)

	return err
}

func addTopologyDomainsToDSYaml(template, labels string) string {
	return strings.ReplaceAll(template, "# - \"--domainlabels=failure-domain/region,failure-domain/zone\"",
		"- \"--domainlabels="+labels+"\"")
}

func oneReplicaDeployYaml(template string) string {
	re := regexp.MustCompile(`(\s+replicas:) \d+`)

	return re.ReplaceAllString(template, `$1 1`)
}

func enableTopologyInTemplate(data string) string {
	return strings.ReplaceAll(data, "--feature-gates=Topology=false", "--feature-gates=Topology=true")
}

func writeDataAndCalChecksum(app *v1.Pod, opt *metav1.ListOptions, f *framework.Framework) (string, error) {
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
	// write data in PVC
	err := writeDataInPod(app, opt, f)
	if err != nil {
		framework.Logf("failed to write data in the pod: %v", err)

		return "", err
	}

	checkSum, err := calculateSHA512sum(f, app, filePath, opt)
	if err != nil {
		framework.Logf("failed to calculate checksum: %v", err)

		return checkSum, err
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		framework.Failf("failed to delete pod: %v", err)
	}

	return checkSum, nil
}

//nolint:gocyclo,gocognit,nestif,cyclop // reduce complexity
func validatePVCClone(
	totalCount int,
	sourcePvcPath, sourceAppPath, clonePvcPath, clonePvcAppPath,
	restoreSCName,
	dataPool string,
	kms kmsConfig,
	validatePVC validateFunc,
	f *framework.Framework,
) {
	var wg sync.WaitGroup
	wgErrs := make([]error, totalCount)
	chErrs := make([]error, totalCount)
	pvc, err := loadPVC(sourcePvcPath)
	if err != nil {
		framework.Failf("failed to load PVC: %v", err)
	}

	label := make(map[string]string)
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		framework.Failf("failed to create PVC: %v", err)
	}
	app, err := loadApp(sourceAppPath)
	if err != nil {
		framework.Failf("failed to load app: %v", err)
	}
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}

	checkSum := ""
	pvc, err = getPersistentVolumeClaim(f.ClientSet, pvc.Namespace, pvc.Name)
	if err != nil {
		framework.Failf("failed to get pvc %v", err)
	}
	if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		checkSum, err = writeDataAndCalChecksum(app, &opt, f)
		if err != nil {
			framework.Failf("failed to calculate checksum: %v", err)
		}
	}
	// validate created backend rbd images
	validateRBDImageCount(f, 1, defaultRBDPool)
	pvcClone, err := loadPVC(clonePvcPath)
	if err != nil {
		framework.Failf("failed to load PVC: %v", err)
	}
	pvcClone.Spec.DataSource.Name = pvc.Name
	pvcClone.Namespace = f.UniqueName
	if restoreSCName != "" {
		pvcClone.Spec.StorageClassName = &restoreSCName
	}

	appClone, err := loadApp(clonePvcAppPath)
	if err != nil {
		framework.Failf("failed to load application: %v", err)
	}
	appClone.Namespace = f.UniqueName
	wg.Add(totalCount)
	// create clone and bind it to an app
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			label := make(map[string]string)
			label[appKey] = name
			a.Labels = label
			opt := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
			}
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			if wgErrs[n] == nil && dataPool != noDataPool {
				wgErrs[n] = checkPVCDataPoolForImageInPool(f, &p, defaultRBDPool, dataPool)
			}
			if wgErrs[n] == nil && kms != noKMS {
				if kms.canGetPassphrase() {
					imageData, sErr := getImageInfoFromPVC(p.Namespace, name, f)
					if sErr != nil {
						wgErrs[n] = fmt.Errorf(
							"failed to get image info for %s namespace=%s volumehandle=%s error=%w",
							name,
							p.Namespace,
							imageData.csiVolumeHandle,
							sErr)
					} else {
						// check new passphrase created
						stdOut, stdErr := kms.getPassphrase(f, imageData.csiVolumeHandle)
						if stdOut != "" {
							framework.Logf("successfully read the passphrase from vault: %s", stdOut)
						}
						if stdErr != "" {
							wgErrs[n] = fmt.Errorf("failed to read passphrase from vault: %s", stdErr)
						}
					}
				}
			}
			if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem && wgErrs[n] == nil {
				filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				var checkSumClone string
				framework.Logf("Calculating checksum clone for filepath %s", filePath)
				checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
				framework.Logf("checksum for clone is %s", checkSumClone)
				if chErrs[n] != nil {
					framework.Logf("Failed calculating checksum clone %s", chErrs[n])
				}
				if checkSumClone != checkSum {
					framework.Logf("checksum didn't match. checksum=%s and checksumclone=%s", checkSum, checkSumClone)
				}
			}
			if wgErrs[n] == nil && validatePVC != nil && kms != noKMS {
				wgErrs[n] = validatePVC(f, &p, &a)
			}
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	failed := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to create PVC (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("creating PVCs failed, %d errors were logged", failed)
	}

	for i, err := range chErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("calculating checksum failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total
	// temporary clone+ total clones
	totalCloneCount := totalCount + totalCount + 1
	validateRBDImageCount(f, totalCloneCount, defaultRBDPool)
	// delete parent pvc
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		framework.Failf("failed to delete PVC: %v", err)
	}

	totalCloneCount = totalCount + totalCount
	validateRBDImageCount(f, totalCloneCount, defaultRBDPool)
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			var imageData imageInfoFromPVC
			var sErr error
			if kms != noKMS {
				if kms.canGetPassphrase() {
					imageData, sErr = getImageInfoFromPVC(p.Namespace, name, f)
					if sErr != nil {
						wgErrs[n] = fmt.Errorf(
							"failed to get image info for %s namespace=%s volumehandle=%s error=%w",
							name,
							p.Namespace,
							imageData.csiVolumeHandle,
							sErr)
					}
				}
			}
			if wgErrs[n] == nil {
				wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
				if wgErrs[n] == nil && kms != noKMS {
					if kms.canGetPassphrase() {
						// check passphrase deleted
						stdOut, _ := kms.getPassphrase(f, imageData.csiVolumeHandle)
						if stdOut != "" {
							wgErrs[n] = fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
						}
					}
					if wgErrs[n] == nil && kms.canVerifyKeyDestroyed() {
						destroyed, msg := kms.verifyKeyDestroyed(f, imageData.csiVolumeHandle)
						if !destroyed {
							wgErrs[n] = fmt.Errorf("passphrased was not destroyed: %s", msg)
						}
					}
				}
			}
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	validateRBDImageCount(f, 0, defaultRBDPool)
}

//nolint:gocyclo,gocognit,nestif,cyclop // reduce complexity
func validatePVCSnapshot(
	totalCount int,
	pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath string,
	kms, restoreKMS kmsConfig, restoreSCName,
	dataPool string, f *framework.Framework,
	isEncryptedPVC validateFunc,
) {
	var wg sync.WaitGroup
	wgErrs := make([]error, totalCount)
	chErrs := make([]error, totalCount)
	err := createRBDSnapshotClass(f)
	if err != nil {
		framework.Failf("failed to create storageclass: %v", err)
	}
	defer func() {
		err = deleteRBDSnapshotClass()
		if err != nil {
			framework.Failf("failed to delete VolumeSnapshotClass: %v", err)
		}
	}()

	pvc, err := loadPVC(pvcPath)
	if err != nil {
		framework.Failf("failed to load PVC: %v", err)
	}
	label := make(map[string]string)
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		framework.Failf("failed to create PVC: %v", err)
	}
	app, err := loadApp(appPath)
	if err != nil {
		framework.Failf("failed to load app: %v", err)
	}
	// write data in PVC
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	checkSum, err := writeDataAndCalChecksum(app, &opt, f)
	if err != nil {
		framework.Failf("failed to calculate checksum: %v", err)
	}
	validateRBDImageCount(f, 1, defaultRBDPool)
	snap := getSnapshot(snapshotPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name

	wg.Add(totalCount)
	// create snapshot
	for i := 0; i < totalCount; i++ {
		go func(n int, s snapapi.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = createSnapshot(&s, deployTimeout)
			if wgErrs[n] == nil && kms != noKMS {
				if kms.canGetPassphrase() {
					content, sErr := getVolumeSnapshotContent(s.Namespace, s.Name)
					if sErr != nil {
						wgErrs[n] = fmt.Errorf(
							"failed to get snapshotcontent for %s in namespace %s: %w",
							s.Name,
							s.Namespace,
							sErr)
					} else {
						// check new passphrase created
						_, stdErr := kms.getPassphrase(f, *content.Status.SnapshotHandle)
						if stdErr != "" {
							wgErrs[n] = fmt.Errorf("failed to read passphrase from vault: %s", stdErr)
						}
					}
				}
			}
			wg.Done()
		}(i, snap)
	}
	wg.Wait()

	failed := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to create snapshot (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("creating snapshots failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total snaps
	validateRBDImageCount(f, totalCount+1, defaultRBDPool)
	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		framework.Failf("failed to load PVC: %v", err)
	}
	appClone, err := loadApp(appClonePath)
	if err != nil {
		framework.Failf("failed to load application: %v", err)
	}
	pvcClone.Namespace = f.UniqueName
	appClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)
	if restoreSCName != "" {
		pvcClone.Spec.StorageClassName = &restoreSCName
	}

	// create multiple PVC from same snapshot
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			label := make(map[string]string)
			label[appKey] = name
			a.Labels = label
			opt := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
			}
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			if wgErrs[n] == nil && restoreKMS != noKMS {
				if restoreKMS.canGetPassphrase() {
					imageData, sErr := getImageInfoFromPVC(p.Namespace, name, f)
					if sErr != nil {
						wgErrs[n] = fmt.Errorf(
							"failed to get image info for %s namespace=%s volumehandle=%s error=%w",
							name,
							p.Namespace,
							imageData.csiVolumeHandle,
							sErr)
					} else {
						// check new passphrase created
						_, stdErr := restoreKMS.getPassphrase(f, imageData.csiVolumeHandle)
						if stdErr != "" {
							wgErrs[n] = fmt.Errorf("failed to read passphrase from vault: %s", stdErr)
						}
					}
				}
				wgErrs[n] = isEncryptedPVC(f, &p, &a)
			}
			if wgErrs[n] == nil {
				filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				var checkSumClone string
				framework.Logf("calculating checksum clone for filepath %s", filePath)
				checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
				framework.Logf("checksum value for the clone is %s with pod name %s", checkSumClone, name)
				if chErrs[n] != nil {
					framework.Logf("failed to calculte checksum for clone: %s", chErrs[n])
				}
				if checkSumClone != checkSum {
					framework.Logf(
						"checksum value didn't match. checksum=%s and checksumclone=%s",
						checkSum,
						checkSumClone)
				}
			}
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to create PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("creating PVCs and applications failed, %d errors were logged", failed)
	}

	for i, err := range chErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("calculating checksum failed, %d errors were logged", failed)
	}
	// total images in cluster is 1 parent rbd image+ total
	// snaps+ total clones
	totalCloneCount := totalCount + totalCount + 1
	validateRBDImageCount(f, totalCloneCount, defaultRBDPool)
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total
	// snaps
	validateRBDImageCount(f, totalCount+1, defaultRBDPool)
	// create clones from different snapshots and bind it to an
	// app
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			if wgErrs[n] == nil && dataPool != noDataPool {
				wgErrs[n] = checkPVCDataPoolForImageInPool(f, &p, defaultRBDPool, dataPool)
			}

			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to create PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("creating PVCs and applications failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total
	// snaps+ total clones
	totalCloneCount = totalCount + totalCount + 1
	validateRBDImageCount(f, totalCloneCount, defaultRBDPool)
	// delete parent pvc
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		framework.Failf("failed to delete PVC: %v", err)
	}

	// total images in cluster is total snaps+ total clones
	totalSnapCount := totalCount + totalCount
	validateRBDImageCount(f, totalSnapCount, defaultRBDPool)
	wg.Add(totalCount)
	// delete snapshot
	for i := 0; i < totalCount; i++ {
		go func(n int, s snapapi.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			content := &snapapi.VolumeSnapshotContent{}
			var err error
			if kms != noKMS {
				if kms.canGetPassphrase() {
					content, err = getVolumeSnapshotContent(s.Namespace, s.Name)
					if err != nil {
						wgErrs[n] = fmt.Errorf(
							"failed to get snapshotcontent for %s in namespace %s: %w",
							s.Name,
							s.Namespace,
							err)
					}
				}
			}
			if wgErrs[n] == nil {
				wgErrs[n] = deleteSnapshot(&s, deployTimeout)
				if wgErrs[n] == nil && kms != noKMS {
					if kms.canGetPassphrase() {
						// check passphrase deleted
						stdOut, _ := kms.getPassphrase(f, *content.Status.SnapshotHandle)
						if stdOut != "" {
							wgErrs[n] = fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
						}
					}
					if wgErrs[n] == nil && kms.canVerifyKeyDestroyed() {
						destroyed, msg := kms.verifyKeyDestroyed(f, *content.Status.SnapshotHandle)
						if !destroyed {
							wgErrs[n] = fmt.Errorf("passphrased was not destroyed: %s", msg)
						}
					}
				}
			}
			wg.Done()
		}(i, snap)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to delete snapshot (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("deleting snapshots failed, %d errors were logged", failed)
	}

	validateRBDImageCount(f, totalCount, defaultRBDPool)
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		framework.Failf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	// validate created backend rbd images
	validateRBDImageCount(f, 0, defaultRBDPool)
}

// validateController simulates the required operations to validate the
// controller.
// Controller will generates the omap data when the PV is created.
// for that we need to do below operations
// Create PVC with Retain policy
// Store the PVC and PV kubernetes objects so that we can create static
// binding between PVC-PV
// Delete the omap data created for PVC
// Create the static PVC and PV and let controller regenerate the omap
// Mount the PVC to application (NodeStage/NodePublish should work)
// Resize the PVC
// Delete the Application and PVC.
func validateController(
	f *framework.Framework,
	pvcPath, appPath, scPath string,
	scOptions, scParams map[string]string,
) error {
	size := "1Gi"
	poolName := defaultRBDPool
	expandSize := "10Gi"
	var err error
	// create storageclass with retain
	err = createRBDStorageClass(f.ClientSet, f, defaultSCName, scOptions, scParams,
		retainPolicy)
	if err != nil {
		return fmt.Errorf("failed to create storageclass: %w", err)
	}

	// create pvc
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}
	resizePvc := pvc.DeepCopy()
	resizePvc.Namespace = f.UniqueName

	pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create PVC: %w", err)
	}
	// get pvc and pv object
	pvc, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get PVC: %w", err)
	}
	// Recreate storageclass with delete policy
	err = deleteResource(scPath)
	if err != nil {
		return fmt.Errorf("failed to delete storageclass: %w", err)
	}
	err = createRBDStorageClass(f.ClientSet, f, defaultSCName, scOptions, scParams,
		deletePolicy)
	if err != nil {
		return fmt.Errorf("failed to create storageclass: %w", err)
	}
	// delete omap data
	err = deleteJournalInfoInPool(f, pvc, poolName)
	if err != nil {
		return err
	}
	// delete pvc and pv
	err = deletePVCAndPV(f.ClientSet, pvc, pv, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete PVC or PV: %w", err)
	}
	// create pvc and pv with application
	pv.Spec.ClaimRef = nil
	pv.Spec.PersistentVolumeReclaimPolicy = deletePolicy
	// unset the resource version as should not be set on objects to be created
	pvc.ResourceVersion = ""
	pv.ResourceVersion = ""
	err = createPVCAndPV(f.ClientSet, pvc, pv)
	if err != nil {
		framework.Failf("failed to create PVC or PV: %v", err)
	}
	// bind PVC to application
	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "resize-pvc"}
	app.Namespace = f.UniqueName
	opt := metav1.ListOptions{
		LabelSelector: "app=resize-pvc",
	}
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	if scParams["encrypted"] == strconv.FormatBool(true) {
		// check encryption
		err = isBlockEncryptedPVC(f, resizePvc, app)
		if err != nil {
			return err
		}
	} else {
		// resize PVC
		err = expandPVCSize(f.ClientSet, resizePvc, expandSize, deployTimeout)
		if err != nil {
			return err
		}
		switch *pvc.Spec.VolumeMode {
		case v1.PersistentVolumeFilesystem:
			err = checkDirSize(app, f, &opt, expandSize)
			if err != nil {
				return err
			}
		case v1.PersistentVolumeBlock:
			err = checkDeviceSize(app, f, &opt, expandSize)
			if err != nil {
				return err
			}
		}
	}
	// delete pvc and storageclass
	err = deletePVCAndApp("", f, resizePvc, app)
	if err != nil {
		return err
	}

	return deleteResource(rbdExamplePath + "storageclass.yaml")
}

// k8sVersionGreaterEquals checks the ServerVersion of the Kubernetes cluster
// and compares it to the major.minor version passed. In case the version of
// the cluster is equal or higher to major.minor, `true` is returned, `false`
// otherwise.
// If fetching the ServerVersion of the Kubernetes cluster fails, the calling
// test case is marked as `FAILED` and gets aborted.
//
//nolint:deadcode,unused // Unused code will be used in future.
func k8sVersionGreaterEquals(c kubernetes.Interface, major, minor int) bool {
	v, err := c.Discovery().ServerVersion()
	if err != nil {
		framework.Failf("failed to get server version: %v", err)
		// Failf() marks the case as failure, and returns from the
		// Go-routine that runs the case. This function will not have a
		// return value.
	}

	maj := fmt.Sprintf("%d", major)
	min := fmt.Sprintf("%d", minor)

	return (v.Major > maj) || (v.Major == maj && v.Minor >= min)
}

// waitForJobCompletion polls the status of the given job and waits until the
// jobs has succeeded or until the timeout is hit.
func waitForJobCompletion(c kubernetes.Interface, ns, job string, timeout int) error {
	t := time.Duration(timeout) * time.Minute
	start := time.Now()

	framework.Logf("waiting for Job %s/%s to be in state %q", ns, job, batch.JobComplete)

	return wait.PollUntilContextTimeout(context.TODO(), poll, t, true, func(ctx context.Context) (bool, error) {
		j, err := c.BatchV1().Jobs(ns).Get(ctx, job, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get Job: %w", err)
		}

		if j.Status.CompletionTime != nil {
			// Job has successfully completed
			return true, nil
		}

		framework.Logf(
			"Job %s/%s has not completed yet (%d seconds elapsed)",
			ns, job, int(time.Since(start).Seconds()))

		return false, nil
	})
}

// kubectlAction is used to tell retryKubectlInput() what action needs to be
// done.
type kubectlAction string

const (
	// kubectlCreate tells retryKubectlInput() to run "create".
	kubectlCreate = kubectlAction("create")
	// kubectlDelete tells retryKubectlInput() to run "delete".
	kubectlDelete = kubectlAction("delete")
)

// String returns the string format of the kubectlAction, this is automatically
// used when formatting strings with %s or %q.
func (ka kubectlAction) String() string {
	return string(ka)
}

// retryKubectlInput takes a namespace and action telling kubectl what to do,
// it then feeds data through stdin to the process. This function retries until
// no error occurred, or the timeout passed.
func retryKubectlInput(namespace string, action kubectlAction, data string, t int, args ...string) error {
	timeout := time.Duration(t) * time.Minute
	framework.Logf("waiting for kubectl (%s -f args %s) to finish", action, args)
	start := time.Now()

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		cmd := []string{}
		if len(args) != 0 {
			cmd = append(cmd, strings.Join(args, ""))
		}
		cmd = append(cmd, []string{string(action), "-f", "-"}...)

		_, err := e2ekubectl.RunKubectlInput(namespace, data, cmd...)
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if action == kubectlCreate && isAlreadyExistsCLIError(err) {
				return true, nil
			}
			if action == kubectlDelete && isNotFoundCLIError(err) {
				return true, nil
			}
			framework.Logf(
				"will run kubectl (%s) args (%s) again (%d seconds elapsed)",
				action,
				args,
				int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to run kubectl: %w", err)
		}

		return true, nil
	})
}

// retryKubectlFile takes a namespace and action telling kubectl what to do
// with the passed filename and arguments. This function retries until no error
// occurred, or the timeout passed.
func retryKubectlFile(namespace string, action kubectlAction, filename string, t int, args ...string) error {
	timeout := time.Duration(t) * time.Minute
	framework.Logf("waiting for kubectl (%s -f %q args %s) to finish", action, filename, args)
	start := time.Now()

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		cmd := []string{}
		if len(args) != 0 {
			cmd = append(cmd, strings.Join(args, ""))
		}
		cmd = append(cmd, []string{string(action), "-f", filename}...)

		_, err := e2ekubectl.RunKubectl(namespace, cmd...)
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if action == kubectlCreate && isAlreadyExistsCLIError(err) {
				return true, nil
			}
			if action == kubectlDelete && isNotFoundCLIError(err) {
				return true, nil
			}
			framework.Logf(
				"will run kubectl (%s -f %q args %s) again (%d seconds elapsed)",
				action,
				filename,
				args,
				int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to run kubectl: %w", err)
		}

		return true, nil
	})
}

// retryKubectlArgs takes a namespace and action telling kubectl what to do
// with the passed arguments. This function retries until no error occurred, or
// the timeout passed.
//
//nolint:unparam // retryKubectlArgs will be used with kubectlDelete arg later on.
func retryKubectlArgs(namespace string, action kubectlAction, t int, args ...string) error {
	timeout := time.Duration(t) * time.Minute
	args = append([]string{string(action)}, args...)
	framework.Logf("waiting for kubectl (%s args) to finish", args)
	start := time.Now()

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		_, err := e2ekubectl.RunKubectl(namespace, args...)
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			if action == kubectlCreate && isAlreadyExistsCLIError(err) {
				return true, nil
			}
			if action == kubectlDelete && isNotFoundCLIError(err) {
				return true, nil
			}
			framework.Logf(
				"will run kubectl (%s) again (%d seconds elapsed)",
				args,
				int(time.Since(start).Seconds()))

			return false, fmt.Errorf("failed to run kubectl: %w", err)
		}

		return true, nil
	})
}

// rwopSupported indicates that a test using RWOP is expected to succeed. If
// the accessMode is reported as invalid, rwopSupported will be set to false.
var rwopSupported = true

// rwopMayFail returns true if the accessMode is not valid. k8s v1.22 requires
// a feature gate, which might not be set. In case the accessMode is invalid,
// the featuregate is not set, and testing RWOP is not possible.
func rwopMayFail(err error) bool {
	if !rwopSupported {
		return true
	}

	if strings.Contains(err.Error(), `invalid: spec.accessModes: Unsupported value: "ReadWriteOncePod"`) {
		rwopSupported = false
	}

	return !rwopSupported
}

// getConfigFile returns the config file path at the preferred location if it
// exists there. Returns the fallback location otherwise.
func getConfigFile(filename, preferred, fallback string) string {
	configFile := preferred + filename
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		configFile = fallback + filename
	}

	return configFile
}
