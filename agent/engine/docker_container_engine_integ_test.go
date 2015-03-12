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

package engine

import (
	"os"
	"testing"
)

func TestPullLibraryImage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	dgc, err := NewDockerGoClient()
	if err != nil {
		t.Errorf("Unable to create client: %v", err)
	}

	for _, image := range []string{"busybox", "library/busybox:latest", "busybox:latest"} {
		err = dgc.PullImage(image)
		if err != nil {
			t.Errorf("Error pulling library image: %v", err)
		}
	}
}
