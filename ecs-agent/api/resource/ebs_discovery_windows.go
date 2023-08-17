//go:build windows
// +build windows

package resource

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	volumeDiscoveryTimeoutDuration = 5 * time.Second
)

func ConfirmEBSVolumeIsAttached(ctx context.Context, deviceName string, volumeID string) error {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, volumeDiscoveryTimeoutDuration)
	defer cancel()
	commandArguments := []string{"diskdrive", "where", fmt.Sprintf("SerialNumber like '%%%s%%'", volumeId), "get", "SerialNumber"}
	output, err := exec.CommandContext(ctxWithTimeout, "wmic", commandArguments...).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run wmic: %s", string(output))
	}

	parsedOutput, err := parseExecutableOutput(output)
	if err != nil {
		log.Fatal(err)
		return errors.Wrapf(err, "failed to parse wmic output for volumeID: %s", volumeID)
	}

	log.Info(fmt.Sprintf("cound volume with parsed information as:%s", parsedOutput))

	if !strings.Contains(parsedOutput, volumeId) {
		log.Fatal(fmt.Sprintf("could not find volume with ID:%s", volumeId))
	}

	return nil
}

// parseExecutableOutput parses the output of `wmic` and returns the volume SerialNumber.
func parseExecutableOutput(output []byte) (string, error) {
	// The output of the wmic diskdrive where "SerialNumber like '%vol0123456789abdcef0%'" get SerialNumber,DeviceID,Description command looks like the following:
	// SerialNumber
	// vol0123456789abdcef0_00000001. - There is an 8-digit numeric identifier attached in the results.
	out := string(output)
	volumeInfo := strings.Fields(out)
	if len(volumeInfo) != 2 {
		return "", errors.New(fmt.Sprint("cannot find the volume ID. Encountered error message: %s", out))
	}
	return volumeInfo[1], nil
}
