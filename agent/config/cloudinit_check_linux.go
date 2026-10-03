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

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/cihub/seelog"
)

// CloudInitResultFilePath is the path to cloud-init's result.json file which records
// the datasource used during instance boot.
const CloudInitResultFilePath = "/var/lib/cloud/data/result.json"

// cloudInitResult represents the structure of cloud-init's result.json file.
type cloudInitResult struct {
	V1 struct {
		Datasource string `json:"datasource"`
	} `json:"v1"`
}

// CheckCloudInitFailure detects if cloud-init failed to execute userdata due to
// IMDS unavailability during boot. Returns a non-nil error only when both
// conditions are met: DataSourceNone in result.json AND non-empty userdata on
// IMDS. Any failure to perform the check returns nil (best-effort, fail-open).
func CheckCloudInitFailure(ec2Client ec2.EC2MetadataClient, resultFilePath string) error {
	data, err := os.ReadFile(resultFilePath)
	if err != nil {
		seelog.Warnf("Unable to read cloud-init result file at %s, skipping cloud-init check: %v",
			resultFilePath, err)
		return nil
	}

	var result cloudInitResult
	if err := json.Unmarshal(data, &result); err != nil {
		seelog.Warnf("Unable to parse cloud-init result file at %s, skipping cloud-init check: %v",
			resultFilePath, err)
		return nil
	}

	if result.V1.Datasource != "DataSourceNone" {
		seelog.Infof("cloud-init datasource is %s, cloud-init check passed", result.V1.Datasource)
		return nil
	}

	userData, err := ec2Client.GetUserData()
	if err != nil {
		var statusErr interface{ HTTPStatusCode() int }
		if errors.As(err, &statusErr) && statusErr.HTTPStatusCode() == http.StatusNotFound {
			seelog.Infof("cloud-init used DataSourceNone but no userdata is configured for this instance, skipping check")
		} else {
			seelog.Infof("cloud-init used DataSourceNone; unable to verify if userdata exists on IMDS, skipping check: %v", err)
		}
		return nil
	}
	if userData == "" {
		seelog.Infof("cloud-init used DataSourceNone but userdata is empty, skipping check")
		return nil
	}

	return fmt.Errorf("cloud-init used DataSourceNone and userdata exists on IMDS but was never executed")
}
