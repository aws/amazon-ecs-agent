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
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cihub/seelog"
)

const (
	LOGLEVEL_ENV_VAR           = "ECS_LOGLEVEL"
	LOGFILE_ENV_VAR            = "ECS_LOGFILE"
	LOG_ROLLOVER_TYPE_ENV_VAR  = "ECS_LOG_ROLLOVER_TYPE"
	LOG_OUTPUT_FORMAT_ENV_VAR  = "ECS_LOG_OUTPUT_FORMAT"
	LOG_MAX_FILE_SIZE_ENV_VAR  = "ECS_LOG_MAX_FILE_SIZE_MB"
	LOG_MAX_ROLL_COUNT_ENV_VAR = "ECS_LOG_MAX_ROLL_COUNT"

	DEFAULT_LOGLEVEL               = "info"
	DEFAULT_ROLLOVER_TYPE          = "date"
	DEFAULT_OUTPUT_FORMAT          = "logfmt"
	DEFAULT_MAX_FILE_SIZE  float64 = 10
	DEFAULT_MAX_ROLL_COUNT int     = 24
)

type logConfig struct {
	logfile       string
	level         string
	rolloverType  string
	outputFormat  string
	maxRollCount  int
	maxFileSizeMB float64
	sync.Mutex
}

var config *logConfig

func logfmtFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return fmt.Sprintf(`level=%s time=%s msg=%q module=%s
`, level.String(), time.Now().UTC().Format(time.RFC3339), message, context.FileName())
	}
}

func jsonFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return fmt.Sprintf(`{"level": %q, "time": %q, "msg": %q, "module": %q}
`, level.String(), time.Now().UTC().Format(time.RFC3339), message, context.FileName())
	}
}

func reloadConfig() {
	logger, err := seelog.LoggerFromConfigAsString(seelogConfig())
	if err == nil {
		seelog.ReplaceLogger(logger)
	} else {
		seelog.Error(err)
	}
}

func seelogConfig() string {
	c := `
<seelog type="asyncloop" minlevel="` + config.level + `">
	<outputs formatid="` + config.outputFormat + `">
		<console />`
	c += platformLogConfig()
	if config.logfile != "" {
		if config.rolloverType == "size" {
			c += `
		<rollingfile filename="` + config.logfile + `" type="size"
		 maxsize="` + strconv.Itoa(int(config.maxFileSizeMB*1000000)) + `" archivetype="none" maxrolls="` + strconv.Itoa(config.maxRollCount) + `" />`
		} else {
			c += `
		<rollingfile filename="` + config.logfile + `" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="` + strconv.Itoa(config.maxRollCount) + `" />`
		}
	}
	c += `
	</outputs>
	<formats>
		<format id="logfmt" format="%EcsAgentLogfmt" />
		<format id="json" format="%EcsAgentJson" />
	</formats>
</seelog>`
	return c
}

// SetLevel sets the log level for logging
func SetLevel(logLevel string) {
	levels := map[string]string{
		"debug": "debug",
		"info":  "info",
		"warn":  "warn",
		"error": "error",
		"crit":  "critical",
		"none":  "off",
	}
	parsedLevel, ok := levels[strings.ToLower(logLevel)]

	if ok {
		config.Lock()
		defer config.Unlock()
		config.level = parsedLevel
		reloadConfig()
	}
}

// GetLevel gets the log level
func GetLevel() string {
	config.Lock()
	defer config.Unlock()

	return config.level
}

func init() {
	config = &logConfig{
		logfile:       os.Getenv(LOGFILE_ENV_VAR),
		level:         DEFAULT_LOGLEVEL,
		rolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		maxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		maxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}

	SetLevel(os.Getenv(LOGLEVEL_ENV_VAR))
	if rolloverType := os.Getenv(LOG_ROLLOVER_TYPE_ENV_VAR); rolloverType != "" {
		config.rolloverType = rolloverType
	}
	if outputFormat := os.Getenv(LOG_OUTPUT_FORMAT_ENV_VAR); outputFormat != "" {
		config.outputFormat = outputFormat
	}
	if maxRollCount := os.Getenv(LOG_MAX_ROLL_COUNT_ENV_VAR); maxRollCount != "" {
		i, err := strconv.Atoi(maxRollCount)
		if err == nil {
			config.maxRollCount = i
		} else {
			seelog.Error("Invalid value for "+LOG_MAX_ROLL_COUNT_ENV_VAR, err)
		}
	}
	if maxFileSizeMB := os.Getenv(LOG_MAX_FILE_SIZE_ENV_VAR); maxFileSizeMB != "" {
		f, err := strconv.ParseFloat(maxFileSizeMB, 64)
		if err == nil {
			config.maxFileSizeMB = f
		} else {
			seelog.Error("Invalid value for "+LOG_MAX_FILE_SIZE_ENV_VAR, err)
		}
	}

	if err := seelog.RegisterCustomFormatter("EcsAgentLogfmt", logfmtFormatter); err != nil {
		seelog.Error(err)
	}
	if err := seelog.RegisterCustomFormatter("EcsAgentJson", jsonFormatter); err != nil {
		seelog.Error(err)
	}

	registerPlatformLogger()
	reloadConfig()
}
