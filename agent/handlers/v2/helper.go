// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v2

import (
	"net"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/pkg/errors"
)

func getTaskARNByRequest(r *http.Request, state dockerstate.TaskEngineState) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", errors.Errorf("unable to parse request's ip address: %v", err)
	}

	// Get task arn for the request by looking up the ip address
	taskARN, ok := state.GetTaskByIPAddress(ip)
	if !ok {
		return "", errors.Errorf("unable to associate '%s' with task", ip)
	}

	return taskARN, nil
}
