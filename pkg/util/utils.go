package util

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"k8s.io/klog"
)

// Timeout for Command execution
// exporting Timeout to make it configurable
var Timeout int

// ExecCommand is a wrapper over exec.Command with timeout
func ExecCommand(command string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(Timeout)*time.Second)
	defer cancel()
	klog.V(4).Infof("exec %s %s", command, args)

	cmd := exec.Command(command, args...) // #nosec
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("command timed out")
	}

	if err != nil {
		return nil, fmt.Errorf("command exited with non-zero code: %v", err)
	}
	return out, err
}
