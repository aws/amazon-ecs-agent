// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package updater

import (
	"errors"
	"net/url"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
)

func validateUpdateInfo(updateInfo *ecsacs.UpdateInfo) error {
	if updateInfo == nil {
		return errors.New("no update info given")
	}
	if updateInfo.Location == nil {
		return errors.New("no location given")
	}
	if updateInfo.Signature == nil {
		return errors.New("no signature given")
	}

	return locationValidation(*updateInfo.Location)
}

const validUpdateHost = "s3.amazonaws.com"
const validUpdateBucket = "amazon-ecs-agent"

// locationValidation validates that the update location points to the
// amazon-ecs-agent update bucket
func locationValidation(location string) error {
	parsed, err := url.Parse(location)
	if err != nil {
		return err
	}

	if parsed.Host != validUpdateHost {
		return errors.New("invalid update host")
	}
	if !strings.HasPrefix(parsed.Path, "/"+validUpdateBucket+"/") {
		return errors.New("invalid update bucket")
	}

	return nil
}
