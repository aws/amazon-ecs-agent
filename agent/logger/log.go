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
	"sort"
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
	RolloverType  string
	MaxRollCount  int
	MaxFileSizeMB float64
	logfile       string
	level         string
	outputFormat  string
	lock          sync.Mutex
}

var Config *logConfig

func logfmtFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		cc, ok := context.CustomContext().(map[string]string)
		var customContext string
		if ok && len(cc) > 0 {
			var sortedContext []string
			for k, v := range cc {
				sortedContext = append(sortedContext, k+"="+v)
			}
			sort.Strings(sortedContext)
			customContext = " " + strings.Join(sortedContext, " ")
		}
		return fmt.Sprintf(`level=%s time=%s msg=%q module=%s%s
`, level.String(), context.CallTime().UTC().Format(time.RFC3339), message, context.FileName(), customContext)
	}
}

func jsonFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		cc, ok := context.CustomContext().(map[string]string)
		var customContext string
		if ok && len(cc) > 0 {
			for k, v := range cc {
				customContext += fmt.Sprintf(", %q: %q", k, v)
			}
		}
		return fmt.Sprintf(`{"level": %q, "time": %q, "msg": %q, "module": %q%s}
`, level.String(), context.CallTime().UTC().Format(time.RFC3339), message, context.FileName(), customContext)
	}
}

func seelogConfig() string {
	c := `
<seelog type="asyncloop" minlevel="` + Config.level + `">
	<outputs formatid="` + Config.outputFormat + `">
		<console />`
	c += platformLogConfig()
	if Config.logfile != "" {
		if Config.RolloverType == "size" {
			c += `
		<rollingfile filename="` + Config.logfile + `" type="size"
		 maxsize="` + strconv.Itoa(int(Config.MaxFileSizeMB*1000000)) + `" archivetype="none" maxrolls="` + strconv.Itoa(Config.MaxRollCount) + `" />`
		} else {
			c += `
		<rollingfile filename="` + Config.logfile + `" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="` + strconv.Itoa(Config.MaxRollCount) + `" />`
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
		Config.lock.Lock()
		defer Config.lock.Unlock()
		Config.level = parsedLevel
		reloadMainConfig()
	}
}

// GetLevel gets the log level
func GetLevel() string {
	Config.lock.Lock()
	defer Config.lock.Unlock()

	return Config.level
}

func InitLogger() seelog.LoggerInterface {
	logger, err := seelog.LoggerFromConfigAsString(seelogConfig())
	if err != nil {
		seelog.Errorf("Error creating seelog logger: %s", err)
		return seelog.Default
	}
	return logger
}

func reloadMainConfig() {
	logger, err := seelog.LoggerFromConfigAsString(seelogConfig())
	if err == nil {
		seelog.ReplaceLogger(logger)
	} else {
		seelog.Error(err)
	}
}

func init() {
	Config = &logConfig{
		logfile:       os.Getenv(LOGFILE_ENV_VAR),
		level:         DEFAULT_LOGLEVEL,
		RolloverType:  DEFAULT_ROLLOVER_TYPE,
		outputFormat:  DEFAULT_OUTPUT_FORMAT,
		MaxFileSizeMB: DEFAULT_MAX_FILE_SIZE,
		MaxRollCount:  DEFAULT_MAX_ROLL_COUNT,
	}

	if level := os.Getenv(LOGLEVEL_ENV_VAR); level != "" {
		SetLevel(level)
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

	if err := seelog.RegisterCustomFormatter("EcsAgentLogfmt", logfmtFormatter); err != nil {
		seelog.Error(err)
	}
	if err := seelog.RegisterCustomFormatter("EcsAgentJson", jsonFormatter); err != nil {
		seelog.Error(err)
	}
	registerPlatformLogger()
	seelog.ReplaceLogger(InitLogger())
}
