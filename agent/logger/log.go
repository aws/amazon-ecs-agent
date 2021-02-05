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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cihub/seelog"
)

const (
	LOGLEVEL_ENV_VAR             = "ECS_LOGLEVEL"
	LOGLEVEL_ON_INSTANCE_ENV_VAR = "ECS_LOGLEVEL_ON_INSTANCE"
	LOGFILE_ENV_VAR              = "ECS_LOGFILE"
	LOG_DRIVER_ENV_VAR           = "ECS_LOG_DRIVER"
	LOG_ROLLOVER_TYPE_ENV_VAR    = "ECS_LOG_ROLLOVER_TYPE"
	LOG_OUTPUT_FORMAT_ENV_VAR    = "ECS_LOG_OUTPUT_FORMAT"
	LOG_MAX_FILE_SIZE_ENV_VAR    = "ECS_LOG_MAX_FILE_SIZE_MB"
	LOG_MAX_ROLL_COUNT_ENV_VAR   = "ECS_LOG_MAX_ROLL_COUNT"

	logFmt  = "logfmt"
	jsonFmt = "json"

	DEFAULT_LOGLEVEL                         = "info"
	DEFAULT_LOGLEVEL_WHEN_DRIVER_SET         = "off"
	DEFAULT_ROLLOVER_TYPE                    = "date"
	DEFAULT_OUTPUT_FORMAT                    = logFmt
	DEFAULT_MAX_FILE_SIZE            float64 = 10
	DEFAULT_MAX_ROLL_COUNT           int     = 24
)

type logConfig struct {
	RolloverType  string
	MaxRollCount  int
	MaxFileSizeMB float64
	logfile       string
	driverLevel   string
	instanceLevel string
	outputFormat  string
	lock          sync.Mutex
}

var Config *logConfig

func ecsMsgFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)
		// temporary measure to make this change backwards compatible as we update to structured logs
		if strings.HasPrefix(message, structuredTxtFormatPrefix) {
			message = strings.TrimPrefix(message, structuredTxtFormatPrefix)
			buf.WriteString(message)
		} else if strings.HasPrefix(message, structuredJsonFormatPrefix) {
			message = strings.TrimPrefix(message, structuredJsonFormatPrefix)
			message = strings.TrimRight(message, ",")
			buf.WriteByte('{')
			buf.WriteString(message)
			buf.WriteByte('}')
		} else {
			buf.WriteString(message)
		}
		return buf.String()
	}
}

func logfmtFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)
		buf.WriteString("level=")
		buf.WriteString(level.String())
		buf.WriteByte(' ')
		buf.WriteString("time=")
		buf.WriteString(context.CallTime().UTC().Format(time.RFC3339))
		buf.WriteByte(' ')
		// temporary measure to make this change backwards compatible as we update to structured logs
		if strings.HasPrefix(message, structuredTxtFormatPrefix) {
			message = strings.TrimPrefix(message, structuredTxtFormatPrefix)
			buf.WriteString(message)
		} else {
			buf.WriteString("msg=")
			buf.WriteString(fmt.Sprintf("%q", message))
			buf.WriteByte(' ')
			buf.WriteString("module=")
			buf.WriteString(context.FileName())
		}
		buf.WriteByte('\n')
		return buf.String()
	}
}

func jsonFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)
		buf.WriteString(`{"level":"`)
		buf.WriteString(level.String())
		buf.WriteString(`","time":"`)
		buf.WriteString(context.CallTime().UTC().Format(time.RFC3339))
		buf.WriteString(`",`)
		// temporary measure to make this change backwards compatible as we update to structured logs
		if strings.HasPrefix(message, structuredJsonFormatPrefix) {
			message = strings.TrimPrefix(message, structuredJsonFormatPrefix)
			message = strings.TrimRight(message, ",")
			buf.WriteString(message)
			buf.WriteByte('}')
		} else {
			buf.WriteString(`"msg":`)
			buf.WriteString(fmt.Sprintf("%q", message))
			buf.WriteString(`,"module":"`)
			buf.WriteString(context.FileName())
			buf.WriteString(`"}`)
		}
		buf.WriteByte('\n')
		return buf.String()
	}
}

func reloadConfig() {
	logger, err := seelog.LoggerFromConfigAsString(seelogConfig())
	if err != nil {
		seelog.Error(err)
		return
	}
	setGlobalLogger(logger, Config.outputFormat)
}

func seelogConfig() string {
	c := `
<seelog type="asyncloop">
	<outputs formatid="` + Config.outputFormat + `">
		<filter levels="` + getLevelList(Config.driverLevel) + `">
			<console />`
	c += platformLogConfig()
	c += `
		</filter>`
	if Config.logfile != "" {
		c += `
		<filter levels="` + getLevelList(Config.instanceLevel) + `">`
		if Config.RolloverType == "size" {
			c += `
			<rollingfile filename="` + Config.logfile + `" type="size"
			 maxsize="` + strconv.Itoa(int(Config.MaxFileSizeMB*1000000)) + `" archivetype="none" maxrolls="` + strconv.Itoa(Config.MaxRollCount) + `" />`
		} else {
			c += `
			<rollingfile filename="` + Config.logfile + `" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="` + strconv.Itoa(Config.MaxRollCount) + `" />`
		}
		c += `
		</filter>`
	}
	c += `
	</outputs>
	<formats>
		<format id="` + logFmt + `" format="%EcsAgentLogfmt" />
		<format id="` + jsonFmt + `" format="%EcsAgentJson" />
		<format id="windows" format="%EcsMsg" />
	</formats>
</seelog>`

	return c
}

func getLevelList(fileLevel string) string {
	levelLists := map[string]string{
		"debug":    "debug,info,warn,error,critical",
		"info":     "info,warn,error,critical",
		"warn":     "warn,error,critical",
		"error":    "error,critical",
		"critical": "critical",
		"off":      "off",
	}
	return levelLists[fileLevel]
}

// SetLevel sets the log levels for logging
func SetLevel(driverLogLevel, instanceLogLevel string) {
	levels := map[string]string{
		"debug": "debug",
		"info":  "info",
		"warn":  "warn",
		"error": "error",
		"crit":  "critical",
		"none":  "off",
	}

	parsedDriverLevel, driverOk := levels[strings.ToLower(driverLogLevel)]
	parsedInstanceLevel, instanceOk := levels[strings.ToLower(instanceLogLevel)]

	if instanceOk || driverOk {
		Config.lock.Lock()
		defer Config.lock.Unlock()
		if instanceOk {
			Config.instanceLevel = parsedInstanceLevel
		}
		if driverOk {
			Config.driverLevel = parsedDriverLevel
		}
		reloadConfig()
	}
}

// GetLevel gets the log level
func GetLevel() string {
	Config.lock.Lock()
	defer Config.lock.Unlock()

	return Config.driverLevel
}

func setInstanceLevelDefault() string {
	if logDriver := os.Getenv(LOG_DRIVER_ENV_VAR); logDriver != "" {
		return DEFAULT_LOGLEVEL_WHEN_DRIVER_SET
	}
	if loglevel := os.Getenv(LOGLEVEL_ENV_VAR); loglevel != "" {
		return loglevel
	}
	return DEFAULT_LOGLEVEL
}

func init() {
	Config = &logConfig{
		logfile:       os.Getenv(LOGFILE_ENV_VAR),
		driverLevel:   DEFAULT_LOGLEVEL,
		instanceLevel: setInstanceLevelDefault(),
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}
}

func InitSeelog() {
	if err := seelog.RegisterCustomFormatter("EcsAgentLogfmt", logfmtFormatter); err != nil {
		seelog.Error(err)
	}
	if err := seelog.RegisterCustomFormatter("EcsAgentJson", jsonFormatter); err != nil {
		seelog.Error(err)
	}
	if err := seelog.RegisterCustomFormatter("EcsMsg", ecsMsgFormatter); err != nil {
		seelog.Error(err)
	}

	SetLevel(os.Getenv(LOGLEVEL_ENV_VAR), os.Getenv(LOGLEVEL_ON_INSTANCE_ENV_VAR))

	if RolloverType := os.Getenv(LOG_ROLLOVER_TYPE_ENV_VAR); RolloverType != "" {
		Config.RolloverType = RolloverType
	}
	if outputFormat := os.Getenv(LOG_OUTPUT_FORMAT_ENV_VAR); outputFormat != "" {
		Config.outputFormat = outputFormat
	}
	if MaxRollCount := os.Getenv(LOG_MAX_ROLL_COUNT_ENV_VAR); MaxRollCount != "" {
		i, err := strconv.Atoi(MaxRollCount)
		if err == nil {
			Config.MaxRollCount = i
		} else {
			seelog.Error("Invalid value for "+LOG_MAX_ROLL_COUNT_ENV_VAR, err)
		}
	}
	if MaxFileSizeMB := os.Getenv(LOG_MAX_FILE_SIZE_ENV_VAR); MaxFileSizeMB != "" {
		f, err := strconv.ParseFloat(MaxFileSizeMB, 64)
		if err == nil {
			Config.MaxFileSizeMB = f
		} else {
			seelog.Error("Invalid value for "+LOG_MAX_FILE_SIZE_ENV_VAR, err)
		}
	}

	registerPlatformLogger()
	reloadConfig()
}
