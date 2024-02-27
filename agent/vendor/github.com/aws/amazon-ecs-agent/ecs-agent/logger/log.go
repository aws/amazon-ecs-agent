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
	DEFAULT_TIMESTAMP_FORMAT                 = time.RFC3339
	DEFAULT_MAX_FILE_SIZE            float64 = 10
	DEFAULT_MAX_ROLL_COUNT           int     = 24
	DEFAULT_LOGTO_STDOUT                     = true
)

// Because timestamp format will be called in the custom formatter
// for each log message processed, it should not be handled
// with an explicitly write protected configuration.
var timestampFormat = DEFAULT_TIMESTAMP_FORMAT

// logLevels is the mapping from ECS_LOGLEVEL to Seelog provided levels.
var logLevels = map[string]string{
	"debug": "debug",
	"info":  "info",
	"warn":  "warn",
	"error": "error",
	"crit":  "critical",
	"none":  "off",
}

type logConfig struct {
	RolloverType  string
	MaxRollCount  int
	MaxFileSizeMB float64
	logfile       string
	driverLevel   string
	instanceLevel string
	outputFormat  string
	logToStdout   bool
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
		buf.WriteString(context.CallTime().UTC().Format(timestampFormat))
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
		buf.WriteString(context.CallTime().UTC().Format(timestampFormat))
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
	driverLogChildren := []string{}
	platformLogConfig := platformLogConfig()
	if Config.logToStdout {
		driverLogChildren = append(driverLogChildren, `	<console />`)
	}
	if platformLogConfig != "" {
		driverLogChildren = append(driverLogChildren, platformLogConfig)
	}

	c := `
<seelog type="asyncloop">
	<outputs formatid="` + Config.outputFormat + `">`
	if len(driverLogChildren) > 0 {
		c += `
		<filter levels="` + getLevelList(Config.driverLevel) + `">
		`
		c += strings.Join(driverLogChildren, "\n")
		c += `
		</filter>`
	}
	if Config.logfile != "" {
		c += `
		<filter levels="` + getLevelList(Config.instanceLevel) + `">`
		if Config.RolloverType == "size" {
			c += `
			<rollingfile filename="` + Config.logfile + `" type="size"
			 maxsize="` + strconv.Itoa(int(Config.MaxFileSizeMB*1000000)) + `" archivetype="none" maxrolls="` + strconv.Itoa(Config.MaxRollCount) + `" />`
		} else if Config.RolloverType == "none" {
			c += `
			<file path="` + Config.logfile + `"/>`
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

// SetInstanceLogLevel explicitly sets the log level for instance logs.
func SetInstanceLogLevel(instanceLogLevel string) {
	parsedLevel, ok := logLevels[strings.ToLower(instanceLogLevel)]
	if ok {
		Config.lock.Lock()
		defer Config.lock.Unlock()
		Config.instanceLevel = parsedLevel
		reloadConfig()
	} else {
		seelog.Error("Instance log level mapping not found")
	}
}

// SetDriverLogLevel explicitly sets the log level for a custom driver.
func SetDriverLogLevel(driverLogLevel string) {
	parsedLevel, ok := logLevels[strings.ToLower(driverLogLevel)]
	if ok {
		Config.lock.Lock()
		defer Config.lock.Unlock()
		Config.driverLevel = parsedLevel
		reloadConfig()
	} else {
		seelog.Error("Driver log level mapping not found")
	}
}

// SetConfigLogFile sets the default output file of the logger.
func SetConfigLogFile(logFile string) {
	if logFile != "" {
		Config.lock.Lock()
		defer Config.lock.Unlock()

		Config.logfile = logFile
		reloadConfig()
	} else {
		seelog.Error("Cannot use empty log file")
	}
}

// SetConfigOutputFormat sets the output format of the logger.
// e.g. json, xml, etc.
func SetConfigOutputFormat(outputFormat string) {
	Config.lock.Lock()
	defer Config.lock.Unlock()

	Config.outputFormat = outputFormat
	reloadConfig()
}

// SetConfigMaxFileSizeMB sets the max file size of a log file
// in Megabytes before the logger rotates to a new file.
func SetConfigMaxFileSizeMB(maxSizeInMB float64) {
	if maxSizeInMB > 0 {
		Config.lock.Lock()
		defer Config.lock.Unlock()

		Config.MaxFileSizeMB = maxSizeInMB
		reloadConfig()
	} else {
		seelog.Error("Invalid Max File Size Provided")
	}
}

// SetRolloverType sets the logging rollover constraint.
// This should be either size or date. Logger will roll
// to a new log file based on this constraint.
func SetRolloverType(rolloverType string) {
	if rolloverType == "date" || rolloverType == "size" || rolloverType == "none" {
		Config.lock.Lock()
		defer Config.lock.Unlock()

		Config.RolloverType = rolloverType
		reloadConfig()
	} else {
		seelog.Error("Invalid log rollover type provided")
	}
}

// SetTimestampFormat sets the time formatting
// for custom seelog formatters. It will expect
// a valid time format such as time.RFC3339
// or "2006-01-02T15:04:05.000".
func SetTimestampFormat(format string) {
	if format != "" {
		timestampFormat = format
	}
}

// SetLogToStdout decides whether the logger
// should write to stdout using the <console/> tag
// in addition to logfiles that are set up.
func SetLogToStdout(duplicate bool) {
	Config.lock.Lock()
	defer Config.lock.Unlock()

	Config.logToStdout = duplicate
	reloadConfig()
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
		logToStdout:   DEFAULT_LOGTO_STDOUT,
	}
}

// InitSeelog registers custom logging formats, updates the internal Config struct
// and reloads the global logger. This should only be called once, as external
// callers should use the Config struct over environment variables directly.
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

	if DriverLogLevel := os.Getenv(LOGLEVEL_ENV_VAR); DriverLogLevel != "" {
		SetDriverLogLevel(DriverLogLevel)
	}
	if InstanceLogLevel := os.Getenv(LOGLEVEL_ON_INSTANCE_ENV_VAR); InstanceLogLevel != "" {
		SetInstanceLogLevel(InstanceLogLevel)
	}
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
