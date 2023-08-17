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
	commandArguments := []string{"diskdrive", "where", fmt.Sprintf("SerialNumber like '%%%s%%'", volumeID), "get", "SerialNumber"}
	output, err := exec.CommandContext(ctxWithTimeout, "wmic", commandArguments...).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run wmic: %s", string(output))
	}

	parsedOutput, err := parseExecutableOutput(output)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to parse wmic output for volumeID: %s", volumeID)
		log.Error(err, errorMessage)
		return errors.Wrap(err, errorMessage)
	}

	log.Info(fmt.Sprintf("found volume with parsed information as:%s", parsedOutput))

	if !strings.Contains(parsedOutput, volumeID) {
		errorMessage := fmt.Sprintf("could not find volume with ID:%s", volumeID)
		log.Error(errorMessage)
		return errors.New(errorMessage)
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
		return "", errors.New(fmt.Sprintf("cannot find the volume ID. Encountered error message: %s", out))
	}
	return volumeInfo[1], nil
}
