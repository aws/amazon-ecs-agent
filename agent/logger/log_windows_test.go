// +build windows

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

func TestJSONFormat(t *testing.T) {
	jsonF := jsonFormatter("")
	out := jsonF("This is my log message", seelog.InfoLvl, &LogContextMock{})
	s, ok := out.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"level": "info", "time": "2018-10-01T01:02:03Z", "msg": "This is my log message", "module": "mytestmodule.go"}`, s)
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
	require.JSONEq(t, `{"level": "debug", "time": "2018-10-01T01:02:03Z", "msg": "This is my log message", "module": "mytestmodule.go"}`, s)
}

func TestSeelogConfig_Default(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_DebugLevel(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   "debug",
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="debug,info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_SizeRollover(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  "size",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="10000000" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_SizeRolloverFileSizeChange(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  "size",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: 15,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="15000000" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_SizeRolloverRollCountChange(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  "size",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: 15,
		MaxRollCount:  10,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="15000000" archivetype="none" maxrolls="10" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_JSONOutput(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  "json",
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  10,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="json">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="10" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_NoOnInstanceLog(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL_WHEN_DRIVER_SET,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="off">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_DifferentLevels(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   "warn",
		instanceLevel: "critical",
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_FileLevelDefault(t *testing.T) {
	os.Setenv(LOG_DRIVER_ENV_VAR, "awslogs")
	defer os.Unsetenv(LOG_DRIVER_ENV_VAR)

	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: setInstanceLevelDefault(),
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="off">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
}

func TestSetLevel(t *testing.T) {
	os.Setenv(LOGLEVEL_ON_INSTANCE_ENV_VAR, "crit")
	os.Setenv(LOGLEVEL_ENV_VAR, "debug")

	resetEnv := func() {
		os.Unsetenv(LOGLEVEL_ENV_VAR)
		os.Unsetenv(LOGLEVEL_ON_INSTANCE_ENV_VAR)
	}
	defer resetEnv()

	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: setInstanceLevelDefault(),
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
	SetLevel(os.Getenv(LOGLEVEL_ENV_VAR), os.Getenv(LOGLEVEL_ON_INSTANCE_ENV_VAR))
	require.Equal(t, "debug", Config.driverLevel)
	require.Equal(t, "critical", Config.instanceLevel)

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="debug,info,warn,error,critical">
			<console />
			<custom name="wineventlog" formatid="windows" />
		</filter>
		<filter levels="critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%Msg" />
	</formats>
</seelog>`, c)
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
