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
	"os"
	"testing"

	"github.com/cihub/seelog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mock_seelog "github.com/aws/amazon-ecs-agent/agent/logger/mocks"
)

func init() {
	format := os.Getenv(LOG_OUTPUT_FORMAT_ENV_VAR)
	if format == "" {
		os.Setenv(LOG_OUTPUT_FORMAT_ENV_VAR, logFmt)
	}
	reloadConfig()
}

func globalLoggerBackup() func() {
	loggerMux.RLock()
	seeLoggerBkp := seelog.Current
	structuredLoggerBkp := globalStructuredLogger
	loggerMux.RUnlock()

	return func() {
		loggerMux.Lock()
		defer loggerMux.Unlock()
		seelog.ReplaceLogger(seeLoggerBkp)
		globalStructuredLogger = structuredLoggerBkp
	}
}

func TestNewStructuredLogger(t *testing.T) {
	sl := newStructuredLogger(logFmt)
	assert.Equal(t, defaultStructuredTextFormatter, sl.formatter)

	sl = newStructuredLogger(jsonFmt)
	assert.Equal(t, defaultStructuredJsonFormatter, sl.formatter)
}

// note: this also tests the global structured logger functions
func TestStructuredLogger(t *testing.T) {
	defer globalLoggerBackup()()
	sl := newStructuredLogger(logFmt)
	for _, tc := range []struct {
		expectedLevel seelog.LogLevel
		funcToTest    []func(string, ...Fields)
	}{
		{
			seelog.LogLevel(seelog.TraceLvl),
			[]func(string, ...Fields){
				sl.Trace,
				Trace,
			},
		},
		{
			seelog.LogLevel(seelog.DebugLvl),
			[]func(string, ...Fields){
				sl.Debug,
				Debug,
			},
		},
		{seelog.LogLevel(seelog.InfoLvl),
			[]func(string, ...Fields){
				sl.Info,
				Info,
			},
		},
		{seelog.LogLevel(seelog.WarnLvl),
			[]func(string, ...Fields){
				sl.Warn,
				Warn,
			},
		},
		{seelog.LogLevel(seelog.ErrorLvl),
			[]func(string, ...Fields){
				sl.Error,
				Error,
			},
		},
		{seelog.LogLevel(seelog.CriticalLvl),
			[]func(string, ...Fields){
				sl.Critical,
				Critical,
			},
		},
	} {
		t.Run(tc.expectedLevel.String(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockReceiver := mock_seelog.NewMockCustomReceiver(ctrl)

			seeLog, err := seelog.LoggerFromCustomReceiver(mockReceiver)
			require.NoError(t, err)
			setGlobalLogger(seeLog, logFmt)

			mockReceiver.EXPECT().ReceiveMessage(
				`logger=structured msg="Live long and prosper 🖖🏼" f1="field value with \"quotation\" marks"`,
				tc.expectedLevel,
				gomock.Any(),
			).Times(2)
			mockReceiver.EXPECT().Flush().AnyTimes()
			mockReceiver.EXPECT().Close().AnyTimes()

			for _, logfunc := range tc.funcToTest {
				logfunc("Live long and prosper 🖖🏼", Fields{
					"f1": `field value with "quotation" marks`,
				})
			}
		})
	}
}

func BenchmarkStructuredLogger_Info(b *testing.B) {
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		Info("Live long and prosper 🖖🏼", Fields{
			"field1": "field value 1",
			"field2": "field value 2",
			"field3": "field value 3",
		})
	}
}

func BenchmarkSeelog_Info(b *testing.B) {
	for n := 0; n < b.N; n++ {
		seelog.Infof("Live long and prosper 🖖🏼 %s=%s %s=%s %s=%s",
			"field1", "field value 1",
			"field2", "field value 2",
			"field3", "field value 3")
	}
}
