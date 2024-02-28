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

	conf "github.com/aws/amazon-ecs-agent/ecs-init/config"
	"github.com/cihub/seelog"
)

const (
	LOGLEVEL_ENV_VAR = "ECS_LOGLEVEL"
	defaultLogLevel  = "info"
	outputFmt        = "logfmt"
	rollCount        = 24
)

// logLevels is the mapping from ECS_INIT_LOGLEVEL to Seelog provided levels.
var logLevels = map[string]string{
	"debug": "debug",
	"info":  "info",
	"warn":  "warn",
	"error": "error",
	"crit":  "critical",
	"none":  "off",
}

type logConfig struct {
	level        string
	outputFormat string
	maxRollCount int
	lock         sync.Mutex
}

// config contains config for seelog logger
var config *logConfig

func init() {
	config = &logConfig{
		level:        defaultLogLevel,
		outputFormat: outputFmt,
		maxRollCount: rollCount,
	}
}

// Setup sets the custom logging config
func Setup() {
	if logLevel := os.Getenv(LOGLEVEL_ENV_VAR); logLevel != "" {
		SetLogLevel(logLevel)
	}
	if err := seelog.RegisterCustomFormatter("InitLogfmt", logfmtFormatter); err != nil {
		seelog.Error(err)
	}
	reloadConfig()
}

func logfmtFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return fmt.Sprintf(`level=%s time=%s msg=%q
`, level.String(), context.CallTime().UTC().Format(time.RFC3339), message)
	}
}

func SetLogLevel(logLevel string) {
	parsedLevel, ok := logLevels[strings.ToLower(logLevel)]
	if ok {
		config.lock.Lock()
		defer config.lock.Unlock()
		config.level = parsedLevel
		reloadConfig()
	} else {
		seelog.Error("log level mapping not found")
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
	if conf.InitLogFile() != "" {
		c += `
		<rollingfile filename="` + conf.InitLogFile() + `" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="` + strconv.Itoa(config.maxRollCount) + `" />`
	}
	c += `
	</outputs>
	<formats>
		<format id="logfmt" format="%InitLogfmt" />
	</formats>
</seelog>`
	return c
}
