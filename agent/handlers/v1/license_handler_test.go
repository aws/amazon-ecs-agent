// +build unit

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

package v1

import (
	"net/http"
	"testing"

	mock_http "github.com/aws/amazon-ecs-agent/agent/handlers/mocks/http"
	mock_utils "github.com/aws/amazon-ecs-agent/agent/utils/mocks"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
)

func TestLicenseHandler(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockResponseWriter := mock_http.NewMockResponseWriter(mockCtrl)
	mockLicenseProvider := mock_utils.NewMockLicenseProvider(mockCtrl)

	licenseProvider = mockLicenseProvider

	text := "text here"
	mockLicenseProvider.EXPECT().GetText().Return(text, nil)
	mockResponseWriter.EXPECT().Write([]byte(text))

	LicenseHandler(mockResponseWriter, nil)
}

func TestLicenseHandlerError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockResponseWriter := mock_http.NewMockResponseWriter(mockCtrl)
	mockLicenseProvider := mock_utils.NewMockLicenseProvider(mockCtrl)

	licenseProvider = mockLicenseProvider

	mockLicenseProvider.EXPECT().GetText().Return("", errors.New("test error"))
	mockResponseWriter.EXPECT().WriteHeader(http.StatusInternalServerError)

	LicenseHandler(mockResponseWriter, nil)
}
