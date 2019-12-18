// +build !windows

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

package logger

import (
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/require"
)

func TestLogfmtFormat(t *testing.T) {
	logfmt := logfmtFormatter("")
	out := logfmt("This is my log message", seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=info time=2018-10-01T01:02:03Z msg="This is my log message" module=mytestmodule.go
`, s)
}

func TestLogfmtFormat_context(t *testing.T) {
	logfmt := logfmtFormatter("")
	out := logfmt("This is my log message", seelog.InfoLvl, &LogContextMock{
		context: map[string]string{
			"myID":  "12345",
			"myARN": "arn:12345:/abc",
		},
	})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=info time=2018-10-01T01:02:03Z msg="This is my log message" module=mytestmodule.go myARN=arn:12345:/abc myID=12345
`, s)
}

func TestJSONFormat(t *testing.T) {
	jsonF := jsonFormatter("")
	out := jsonF("This is my log message", seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `
		{
			"level": "info",
			"time": "2018-10-01T01:02:03Z",
			"msg": "This is my log message",
			"module": "mytestmodule.go"
		}`, s)
}

func TestJSONFormat_context(t *testing.T) {
	jsonF := jsonFormatter("")
	out := jsonF("This is my log message", seelog.InfoLvl, &LogContextMock{
		context: map[string]string{
			"myID":  "12345",
			"myARN": "arn:12345:/abc",
		},
	})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `
	{
		"level": "info",
		"time": "2018-10-01T01:02:03Z",
		"msg": "This is my log message",
		"module": "mytestmodule.go",
		"myARN":"arn:12345:/abc",
		"myID":"12345"
	}`, s)
}

func TestLogfmtFormat_debug(t *testing.T) {
	logfmt := logfmtFormatter("")
	out := logfmt("This is my log message", seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.Equal(t, `level=debug time=2018-10-01T01:02:03Z msg="This is my log message" module=mytestmodule.go
`, s)
}

func TestJSONFormat_debug(t *testing.T) {
	jsonF := jsonFormatter("")
	out := jsonF("This is my log message", seelog.DebugLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `
		{
			"level": "debug",
			"time": "2018-10-01T01:02:03Z",
			"msg": "This is my log message",
			"module": "mytestmodule.go"
		}`, s)
}

func TestSeelogConfig_Default(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		level:         DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop" minlevel="info">
	<outputs formatid="logfmt">
		<console />
		<rollingfile filename="foo.log" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_DebugLevel(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		level:         "debug",
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop" minlevel="debug">
	<outputs formatid="logfmt">
		<console />
		<rollingfile filename="foo.log" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_SizeRollover(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		level:         DEFAULT_LOGLEVEL,
		RolloverType:  "size",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop" minlevel="info">
	<outputs formatid="logfmt">
		<console />
		<rollingfile filename="foo.log" type="size"
		 maxsize="10000000" archivetype="none" maxrolls="24" />
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_SizeRolloverFileSizeChange(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		level:         DEFAULT_LOGLEVEL,
		RolloverType:  "size",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: 15,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop" minlevel="info">
	<outputs formatid="logfmt">
		<console />
		<rollingfile filename="foo.log" type="size"
		 maxsize="15000000" archivetype="none" maxrolls="24" />
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_SizeRolloverRollCountChange(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		level:         DEFAULT_LOGLEVEL,
		RolloverType:  "size",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: 15,
		MaxRollCount:  10,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop" minlevel="info">
	<outputs formatid="logfmt">
		<console />
		<rollingfile filename="foo.log" type="size"
		 maxsize="15000000" archivetype="none" maxrolls="10" />
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_JSONOutput(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		level:         DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  "json",
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  10,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop" minlevel="info">
	<outputs formatid="json">
		<console />
		<rollingfile filename="foo.log" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="10" />
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`, c)
}

type LogContextMock struct {
	context map[string]string
}

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
	return l.context
}
