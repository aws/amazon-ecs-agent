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

	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/cihub/seelog"
)

// runHealthcheck runs the Agent's healthcheck
func runHealthcheck() int {
	_, err := http.Get("http://localhost:51678/v1/metadata")
	if err != nil {
		seelog.Warnf("Health check failed with error: %v", err)
		return exitcodes.ExitError
	}
	return exitcodes.ExitSuccess
}
