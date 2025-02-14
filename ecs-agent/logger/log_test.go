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
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	mock_seelog "github.com/aws/amazon-ecs-agent/ecs-agent/logger/mocks"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/require"
)

func TestEcsMsgFormat(t *testing.T) {
	logfmt := ecsMsgFormatter("")
	out := logfmt("This is my log message", seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, "This is my log message", s)
}

func TestEcsMsgFormat_Structured(t *testing.T) {
	logfmt := ecsMsgFormatter("")
	fm := defaultStructuredTextFormatter.Format("This is my log message")
	out := logfmt(fm, seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `msg="This is my log message"`, s)

	fm = defaultStructuredJsonFormatter.Format("This is my log message")
	out = logfmt(fm, seelog.InfoLvl, &LogContextMock{})
	s, ok = out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"msg":"This is my log message"}`, s)
}

func TestLogfmtFormat(t *testing.T) {
	logfmt := logfmtFormatter("")
	out := logfmt("This is my log message", seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=info time=2018-10-01T01:02:03Z msg="This is my log message" module=mytestmodule.go
`, s)
}

func TestLogfmtFormat_Structured(t *testing.T) {
	logfmt := logfmtFormatter("")
	fm := defaultStructuredTextFormatter.Format("This is my log message")
	out := logfmt(fm, seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, "level=info time=2018-10-01T01:02:03Z msg=\"This is my log message\"\n", s)
}

func TestJSONFormat(t *testing.T) {
	jsonF := jsonFormatter("")
	out := jsonF("This is my log message", seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"level": "info", "time": "2018-10-01T01:02:03Z", "msg": "This is my log message", "module": "mytestmodule.go"}`, s)
}

func TestJSONFormat_Structured(t *testing.T) {
	jsonF := jsonFormatter("")
	fm := defaultStructuredJsonFormatter.Format(`This is my log message "escaped"`)
	out := jsonF(fm, seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"level": "info", "time": "2018-10-01T01:02:03Z", "msg": "This is my log message \"escaped\""}`, s)
}

func TestLogfmtFormat_debug(t *testing.T) {
	logfmt := logfmtFormatter("")
	out := logfmt("This is my log message", seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=debug time=2018-10-01T01:02:03Z msg="This is my log message" module=mytestmodule.go
`, s)
}

func TestLogfmtFormat_Structured_debug(t *testing.T) {
	logfmt := logfmtFormatter("")
	fm := defaultStructuredTextFormatter.Format("This is my log message")
	out := logfmt(fm, seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=debug time=2018-10-01T01:02:03Z msg="This is my log message"
`, s)
}

func TestLogfmtFormat_Structured_Timestamp(t *testing.T) {
	SetTimestampFormat("2006-01-02T15:04:05.000")
	defer SetTimestampFormat(DEFAULT_TIMESTAMP_FORMAT)
	logfmt := logfmtFormatter("")
	fm := defaultStructuredTextFormatter.Format("This is my log message")
	out := logfmt(fm, seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=debug time=2018-10-01T01:02:03.000 msg="This is my log message"
`, s)
}

func TestJSONFormat_debug(t *testing.T) {
	jsonF := jsonFormatter("")
	out := jsonF("This is my log message", seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"level": "debug", "time": "2018-10-01T01:02:03Z", "msg": "This is my log message", "module": "mytestmodule.go"}`, s)
}

func TestJSONFormat_Structured_debug(t *testing.T) {
	jsonF := jsonFormatter("")
	fm := defaultStructuredJsonFormatter.Format("This is my log message")
	out := jsonF(fm, seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"level": "debug", "time": "2018-10-01T01:02:03Z", "msg": "This is my log message"}`, s)
}

func TestJSONFormat_Structured_Timestamp(t *testing.T) {
	SetTimestampFormat("2006-01-02T15:04:05.000")
	defer SetTimestampFormat(DEFAULT_TIMESTAMP_FORMAT)
	jsonF := jsonFormatter("")
	fm := defaultStructuredJsonFormatter.Format("This is my log message")
	out := jsonF(fm, seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"level": "debug", "time": "2018-10-01T01:02:03.000", "msg": "This is my log message"}`, s)
}

func TestSetLogLevels(t *testing.T) {
	resetEnv := func() {
		os.Unsetenv(LOGLEVEL_ENV_VAR)
		os.Unsetenv(LOGLEVEL_ON_INSTANCE_ENV_VAR)
		os.Unsetenv(LOG_DRIVER_ENV_VAR)
	}
	resetEnv()

	testcases := []struct {
		name                     string
		logDriver                string
		loglevel                 string
		loglevelInstance         string
		expectedLoglevel         string
		expectedLoglevelInstance string
	}{
		{
			name:                     "nothing set",
			logDriver:                "",
			loglevel:                 "",
			loglevelInstance:         "",
			expectedLoglevel:         "info",
			expectedLoglevelInstance: "info",
		},
		{
			name:                     "only loglevel",
			logDriver:                "",
			loglevel:                 "debug",
			loglevelInstance:         "",
			expectedLoglevel:         "debug",
			expectedLoglevelInstance: "debug",
		},
		{
			name:                     "only on instance",
			logDriver:                "",
			loglevel:                 "",
			loglevelInstance:         "debug",
			expectedLoglevel:         "info",
			expectedLoglevelInstance: "debug",
		},
		{
			name:                     "both levels no driver",
			logDriver:                "",
			loglevel:                 "warn",
			loglevelInstance:         "crit",
			expectedLoglevel:         "warn",
			expectedLoglevelInstance: "critical",
		},
		{
			name:                     "loglevel and driver",
			logDriver:                "journald",
			loglevel:                 "debug",
			loglevelInstance:         "",
			expectedLoglevel:         "debug",
			expectedLoglevelInstance: "off",
		},
		{
			name:                     "loglevel on instance and driver",
			logDriver:                "journald",
			loglevel:                 "",
			loglevelInstance:         "debug",
			expectedLoglevel:         "info",
			expectedLoglevelInstance: "debug",
		},
		{
			name:                     "both levels and driver",
			logDriver:                "journald",
			loglevel:                 "warn",
			loglevelInstance:         "debug",
			expectedLoglevel:         "warn",
			expectedLoglevelInstance: "debug",
		},
		{
			name:                     "only driver",
			logDriver:                "journald",
			loglevel:                 "",
			loglevelInstance:         "",
			expectedLoglevel:         "info",
			expectedLoglevelInstance: "off",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			defer resetEnv()

			os.Setenv(LOGLEVEL_ENV_VAR, test.loglevel)
			os.Setenv(LOGLEVEL_ON_INSTANCE_ENV_VAR, test.loglevelInstance)
			os.Setenv(LOG_DRIVER_ENV_VAR, test.logDriver)

			Config = &logConfig{
				logfile:       "foo.log",
				driverLevel:   DEFAULT_LOGLEVEL,
				instanceLevel: setInstanceLevelDefault(),
				RolloverType:  DEFAULT_ROLLOVER_TYPE,
				outputFormat:  DEFAULT_OUTPUT_FORMAT,
				MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
				MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
			}

			SetDriverLogLevel(test.loglevel)
			require.Equal(t, test.expectedLoglevel, Config.driverLevel)

			SetInstanceLogLevel(test.loglevelInstance)
			require.Equal(t, test.expectedLoglevelInstance, Config.instanceLevel)
		})
	}
}

type LogContextMock struct{}

// Caller's function name.
func (l *LogContextMock) Func() string {
	return ""
}

// Caller's line number.
func (l *LogContextMock) Line() int {
	return 0
}

// Caller's file short path (in slashed form).
func (l *LogContextMock) ShortPath() string {
	return ""
}

// Caller's file full path (in slashed form).
func (l *LogContextMock) FullPath() string {
	return ""
}

// Caller's file name (without path).
func (l *LogContextMock) FileName() string {
	return "mytestmodule.go"
}

// True if the context is correct and may be used.
// If false, then an error in context evaluation occurred and
// all its other data may be corrupted.
func (l *LogContextMock) IsValid() bool {
	return true
}

// Time when log function was called.
func (l *LogContextMock) CallTime() time.Time {
	return time.Date(2018, time.October, 1, 1, 2, 3, 0, time.UTC)
}

// Custom context that can be set by calling logger.SetContext
func (l *LogContextMock) CustomContext() interface{} {
	return map[string]string{}
}

func TestSetCustomLogger(t *testing.T) {
	// Store the original logger to restore it later
	originalLogger := seelog.Current
	defer func() {
		// Restore the original logger
		err := seelog.ReplaceLogger(originalLogger)
		if err != nil {
			t.Errorf("Failed to restore original logger: %v", err)
		}
	}()

	tests := []struct {
		name         string
		logLevel     seelog.LogLevel
		logFunc      func(logger seelog.LoggerInterface, msg string)
		outputFormat string
	}{
		{
			name:         "trace",
			logLevel:     seelog.TraceLvl,
			logFunc:      func(l seelog.LoggerInterface, msg string) { l.Trace(msg) },
			outputFormat: jsonFmt,
		},
		{
			name:         "debug",
			logLevel:     seelog.DebugLvl,
			logFunc:      func(l seelog.LoggerInterface, msg string) { l.Debug(msg) },
			outputFormat: logFmt,
		},
		{
			name:         "info",
			logLevel:     seelog.InfoLvl,
			logFunc:      func(l seelog.LoggerInterface, msg string) { l.Info(msg) },
			outputFormat: "windows",
		},
		{
			name:         "warn",
			logLevel:     seelog.WarnLvl,
			logFunc:      func(l seelog.LoggerInterface, msg string) { l.Warn(msg) },
			outputFormat: jsonFmt,
		},
		{
			name:         "error",
			logLevel:     seelog.ErrorLvl,
			logFunc:      func(l seelog.LoggerInterface, msg string) { l.Error(msg) },
			outputFormat: logFmt,
		},
		{
			name:         "critical",
			logLevel:     seelog.CriticalLvl,
			logFunc:      func(l seelog.LoggerInterface, msg string) { l.Critical(msg) },
			outputFormat: "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReceiver := &(mock_seelog.CustomLoggerReceiver{
				OutputFormat: tt.outputFormat,
			})

			SetCustomReceiver(mockReceiver)

			message := fmt.Sprintf("test %s message", tt.name)
			logger := seelog.Current
			tt.logFunc(logger, message)

			// Use a wait group to ensure we've processed the log
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ { // try for up to 5 seconds
					mockReceiver.Mu.Lock()
					var called bool
					var lastMessage string

					// Check the appropriate field based on log level
					switch tt.logLevel {
					case seelog.TraceLvl:
						called = mockReceiver.TraceCalled
						lastMessage = mockReceiver.LastTraceMessage
					case seelog.DebugLvl:
						called = mockReceiver.DebugCalled
						lastMessage = mockReceiver.LastDebugMessage
					case seelog.InfoLvl:
						called = mockReceiver.InfoCalled
						lastMessage = mockReceiver.LastInfoMessage
					case seelog.WarnLvl:
						called = mockReceiver.WarnCalled
						lastMessage = mockReceiver.LastWarnMessage
					case seelog.ErrorLvl:
						called = mockReceiver.ErrorCalled
						lastMessage = mockReceiver.LastErrorMessage
					case seelog.CriticalLvl:
						called = mockReceiver.CriticalCalled
						lastMessage = mockReceiver.LastCriticalMessage
					}
					mockReceiver.Mu.Unlock()

					if called {
						require.Contains(t, lastMessage, message)
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				t.Errorf("%s method was not called within timeout", tt.name)
			}()

			wg.Wait()
		})
	}
}
