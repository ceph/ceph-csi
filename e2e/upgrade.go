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
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// upgradeCSI deploys a desired ceph-csi release version.
func upgradeCSI(version string) error {
	tempDir := "/tmp/ceph-csi"
	gitRepo := "https://github.com/ceph/ceph-csi.git"
	// clone the desired release branch inside a temporary directory.
	cmd := exec.Command("git", "clone", "--single-branch", "--branch", version, gitRepo, tempDir)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unable to clone repo %s : %w", gitRepo, err)
	}
	err := os.Chdir(tempDir + "/e2e")
	if err != nil {
		return fmt.Errorf("unable to switch directory : %w", err)
	}

	return nil
}

// upgradeAndDeployCSI upgrades the CSI to a specific release.
func upgradeAndDeployCSI(version, testtype string) error {
	if err := upgradeCSI(version); err != nil {
		return fmt.Errorf("failed to upgrade driver %w", err)
	}
	switch testtype {
	case "cephfs":
		deployCephfsPlugin()
	case "rbd":
		deployRBDPlugin()
	default:
		return errors.New("incorrect test type, can be cephfs/rbd")
	}

	return nil
}
