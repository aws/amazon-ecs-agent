//go:build unit
// +build unit

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

package logger

import (
	"testing"

	"github.com/cihub/seelog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mock_seelog "github.com/aws/amazon-ecs-agent/ecs-agent/logger/mocks"
)

func TestGlobal_Init(t *testing.T) {
	gl := getGlobalStructuredLogger()
	assert.NotNil(t, gl)
	assert.Equal(t, defaultStructuredTextFormatter, gl.formatter)
}

func TestSetGlobalLogger(t *testing.T) {
	defer globalLoggerBackup()()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockReceiver := mock_seelog.NewMockCustomReceiver(ctrl)
	mockReceiver.EXPECT().Flush().AnyTimes()
	mockReceiver.EXPECT().Close().AnyTimes()
	seeLog, err := seelog.LoggerFromCustomReceiver(mockReceiver)
	require.NoError(t, err)

	prevGlobalStructuredLogger := getGlobalStructuredLogger()
	setGlobalLogger(seeLog, jsonFmt)

	loggerMux.RLock()
	defer loggerMux.RUnlock()
	assert.Equal(t, seeLog, seelog.Current)
	assert.NotNil(t, globalStructuredLogger)
	assert.NotEqual(t, prevGlobalStructuredLogger, globalStructuredLogger)
}
