// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"strconv"
	"time"

	"github.com/cihub/seelog"
)

const (
	logFileFormat = "/var/log/ecs/ecs-volume-plugin.log"
	logLevel      = "info"
	outputFmt     = "logfmt"
	rollCount     = 24
)

type logConfig struct {
	logfile      string
	level        string
	outputFormat string
	maxRollCount int
}

// config contains config for seelog logger
var config *logConfig

// Setup sets the cusotm logging config
func Setup() {
	config = &logConfig{
		logfile:      logFileFormat,
		level:        logLevel,
		outputFormat: outputFmt,
		maxRollCount: rollCount,
	}

	if err := seelog.RegisterCustomFormatter("VolumePluginLogfmt", logfmtFormatter); err != nil {
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
	if config.logfile != "" {
		c += `
		<rollingfile filename="` + config.logfile + `" type="date"
		 datepattern="2006-01-02-15" archivetype="none" maxrolls="` + strconv.Itoa(config.maxRollCount) + `" />`
	}
	c += `
	</outputs>
	<formats>
		<format id="logfmt" format="%VolumePluginLogfmt" />
	</formats>
</seelog>`
	return c
}
