//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package config

import (
	"errors"
	"os"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

func parseGMSACapability() BooleanDefaultFalse {
	envStatus := utils.ParseBool(os.Getenv("ECS_GMSA_SUPPORTED"), true)
	if envStatus {
		// Check if domain join check override is present
		skipDomainJoinCheck := utils.ParseBool(os.Getenv(envSkipDomainJoinCheck), false)
		if skipDomainJoinCheck {
			seelog.Infof("Skipping domain join validation based on environment override")
			return BooleanDefaultFalse{Value: ExplicitlyEnabled}
		}

		// check if the credentials fetcher socket is created and exists
		// this env variable is set in ecs-init module
		if credentialsfetcherHostDir := os.Getenv("CREDENTIALS_FETCHER_HOST_DIR"); credentialsfetcherHostDir != "" {
			_, err := os.Stat(credentialsfetcherHostDir)
			if err != nil {
				if os.IsNotExist(err) {
					seelog.Errorf("CREDENTIALS_FETCHER_HOST_DIR not found, err: %v", err)
					return BooleanDefaultFalse{Value: ExplicitlyDisabled}
				}
			}

			//skip domain join check if the domainless gMSA is supported by setting the env variable
			domainlessGMSAUser := os.Getenv("CREDENTIALS_FETCHER_SECRET_NAME_FOR_DOMAINLESS_GMSA")
			if domainlessGMSAUser != "" && len(domainlessGMSAUser) > 0 {
				seelog.Info("domainless gMSA support is enabled")
				return BooleanDefaultFalse{Value: ExplicitlyEnabled}
			}

			// returns true if the container instance is domain joined
			// this env variable is set in ecs-init module
			isDomainJoined := utils.ParseBool(os.Getenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE"), false)

			if !isDomainJoined {
				seelog.Error("gMSA on linux requires domain joined instance. Did not find expected env var ECS_DOMAIN_JOINED_LINUX_INSTANCE=true")
				return BooleanDefaultFalse{Value: ExplicitlyDisabled}
			}
			return BooleanDefaultFalse{Value: ExplicitlyEnabled}
		}
	}
	seelog.Debug("env variables to support gMSA are not set")
	return BooleanDefaultFalse{Value: ExplicitlyDisabled}
}

func parseFSxWindowsFileServerCapability() BooleanDefaultFalse {
	return BooleanDefaultFalse{Value: ExplicitlyDisabled}
}

var IsWindows2016 = func() (bool, error) {
	return false, errors.New("unsupported platform")
}

// GetOSFamily returns "LINUX" as operating system family for linux based ecs instances.
func GetOSFamily() string {
	return strings.ToUpper(OSType)
}
