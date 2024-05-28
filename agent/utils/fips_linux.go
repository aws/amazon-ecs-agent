//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

const FIPSModeFilePath = "/proc/sys/crypto/fips_enabled"

// DetectFIPSMode checks if FIPS mode is enabled based on the provided file path.
func DetectFIPSMode(filePath string) bool {
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error while detecting FIPS is enabled or not, err: %v", err))
		return false
	}

	fipsValue := strings.TrimSpace(string(data))
	if fipsValue == "1" {
		logger.Info("FIPS mode detected on the host")
		return true
	}

	logger.Debug(fmt.Sprintf("FIPS mode not enabled. FIPS mode explicitly set to %v", fipsValue))
	return false
}
