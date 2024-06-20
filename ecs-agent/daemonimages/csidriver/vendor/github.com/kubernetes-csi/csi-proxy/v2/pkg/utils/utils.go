package utils

import (
	"fmt"
	"os"
	"os/exec"

	"k8s.io/klog/v2"
)

const MaxPathLengthWindows = 260

func RunPowershellCmd(command string, envs ...string) ([]byte, error) {
	command = fmt.Sprintf("$global:ProgressPreference = 'SilentlyContinue'; %s", command)
	cmd := exec.Command("powershell", "-Mta", "-NoProfile", "-Command", command)
	cmd.Env = append(os.Environ(), envs...)
	klog.V(8).Infof("Executing command: %q", cmd.String())
	out, err := cmd.CombinedOutput()
	return out, err
}
