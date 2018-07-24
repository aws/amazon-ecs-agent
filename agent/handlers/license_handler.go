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

package handlers

import (
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/utils"
)

const licensePath = "/license"

var licenseProvider = utils.NewLicenseProvider()

// licenseHandler creates response for '/license' API.
func licenseHandler(w http.ResponseWriter, h *http.Request) {
	text, err := licenseProvider.GetText()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.Write([]byte(text))
	}
}
