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
	err := upgradeCSI(version)
	if err != nil {
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
