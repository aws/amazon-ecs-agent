// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package config

import (
	"os"
	"testing"
)

func TestDockerUnixSocketWithoutDockerHost(t *testing.T) {

	// Make sure that the env variable is not set
	os.Unsetenv("DOCKER_HOST")

	dockerUnixSocketSourcePath, fromEnv := DockerUnixSocket()

	if dockerUnixSocketSourcePath != "/var/run" {
		t.Error("DockerUnixSocket() should be \"/var/run\"")
	}
	if fromEnv {
		t.Error("DockerUnixSocket() should return the default instead of reading from DOCKER_HOST when DOCKER_HOST isn't set")
	}
}

func TestDockerUnixSocketWithDockerHost(t *testing.T) {

	os.Setenv("DOCKER_HOST", "unix:///foo/bar")

	dockerUnixSocketSourcePath, fromEnv := DockerUnixSocket()
	if dockerUnixSocketSourcePath != "/foo/bar" {
		t.Error("DockerUnixSocket() should be \"/foo/bar\"")
	}
	if !fromEnv {
		t.Error("DockerUnixSocket() should read from envrionment variable DOCKER_HOST, when DOCKER_HOST is set")
	}
}

func TestGetS3BucketMapByRegion(t *testing.T) {

	var cases = []struct {
		region         string
		expectedBucket string
	}{
		{DefaultRegionName, regionToS3BucketURL[DefaultRegionName]},
		{"cn-north-1", regionToS3BucketURL["cn-north-1"]},
		{"missing-region", regionToS3BucketURL[DefaultRegionName]},
	}

	for _, c := range cases {
		t.Run(c.region, func(t *testing.T) {
			bucket := getBaseLocationForRegion(DefaultRegionName)
			if bucket != regionToS3BucketURL[DefaultRegionName] {
				t.Errorf("Region Bucket for default region did not match default. Region bucket returned: " + bucket)
			}
		})
	}
}
