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

package app

import (
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/cihub/seelog"
)

// runHealthcheck runs the Agent's healthcheck
func runHealthcheck(url string, timeout time.Duration) int {
	client := &http.Client{
		Timeout: timeout,
	}
	r, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		seelog.Errorf("error creating healthcheck request: %v", err)
		return exitcodes.ExitError
	}
	resp, err := client.Do(r)
	if err != nil {
		seelog.Errorf("health check [HEAD %s] failed with error: %v", url, err)
		return exitcodes.ExitError
	}
	resp.Body.Close()
	return exitcodes.ExitSuccess
}
