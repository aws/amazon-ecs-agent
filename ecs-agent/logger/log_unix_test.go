//go:build !windows && unit
// +build !windows,unit

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

	"github.com/stretchr/testify/require"
)

const DEFAULT_TEST_LOG_FILE = "foo.log"

func SetDefaultConfig() {
	Config = &logConfig{
		logfile:       DEFAULT_TEST_LOG_FILE,
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: setInstanceLevelDefault(),
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
}

func TestSeelogConfig_Default(t *testing.T) {
	SetDefaultConfig()
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_WithoutLogFile(t *testing.T) {
	Config = &logConfig{
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeeLogConfig_WithoutStdout(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  "none",
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
		logToStdout:   false,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<file path="foo.log"/>
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_DebugLevel(t *testing.T) {
	SetDefaultConfig()
	SetDriverLogLevel("debug")

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="debug,info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="10000000" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="15000000" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="15000000" archivetype="none" maxrolls="10" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="json">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="10" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_JSONNoStdout(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  "json",
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  10,
		logToStdout:   false,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="json">
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="10" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_JSONNoRollover(t *testing.T) {
	Config = &logConfig{
		logfile:       "foo.log",
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: DEFAULT_LOGLEVEL,
		RolloverType:  "none",
		outputFormat:  "json",
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  10,
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="json">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<file path="foo.log"/>
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="off">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfig_DifferentLevels(t *testing.T) {
	SetDefaultConfig()

	SetDriverLogLevel("warn")
	SetInstanceLogLevel("critical")
	defer SetDriverLogLevel(DEFAULT_LOGLEVEL)
	defer SetInstanceLogLevel(setInstanceLevelDefault())

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="off">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSetLogFile(t *testing.T) {
	SetDefaultConfig()
	SetRolloverType("none")
	SetConfigLogFile("bar.log")
	defer SetConfigLogFile("foo.log")

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<file path="bar.log"/>
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSetOutputFormat(t *testing.T) {
	SetDefaultConfig()
	SetConfigOutputFormat("json")
	defer SetConfigOutputFormat(DEFAULT_OUTPUT_FORMAT)

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="json">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSetMaxFileSizeMB(t *testing.T) {
	SetDefaultConfig()
	SetRolloverType("size")
	SetConfigMaxFileSizeMB(5)
	defer SetRolloverType(DEFAULT_ROLLOVER_TYPE)
	defer SetConfigMaxFileSizeMB(DEFAULT_MAX_FILE_SIZE)

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="size"
			 maxsize="5000000" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSetRolloverType(t *testing.T) {
	SetDefaultConfig()
	SetRolloverType("none")
	defer SetRolloverType(DEFAULT_ROLLOVER_TYPE)

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<console />
		</filter>
		<filter levels="info,warn,error,critical">
			<file path="foo.log"/>
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSetLogToStdout(t *testing.T) {
	SetDefaultConfig()
	SetLogToStdout(false)
	defer SetLogToStdout(DEFAULT_LOGTO_STDOUT)

	c := seelogConfig()
	require.Equal(t, `
<seelog type="asyncloop">
	<outputs formatid="logfmt">
		<filter levels="info,warn,error,critical">
			<rollingfile filename="foo.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</filter>
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}
