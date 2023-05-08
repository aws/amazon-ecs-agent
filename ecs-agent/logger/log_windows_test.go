//go:build windows && unit
// +build windows,unit

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

func TestSeelogConfigWindows_Default(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_WithoutLogFile(t *testing.T) {
	Config = &logConfig{
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
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_DebugLevel(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_SizeRollover(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_SizeRolloverFileSizeChange(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_SizeRolloverRollCountChange(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_JSONOutput(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_NoOnInstanceLog(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_DifferentLevels(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}

func TestSeelogConfigWindows_FileLevelDefault(t *testing.T) {
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
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`, c)
}
