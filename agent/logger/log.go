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
	"os"
	"strings"
	"sync"

	log15 "gopkg.in/inconshreveable/log15.v2"
)

const (
	LOGLEVEL_ENV_VAR = "ECS_LOGLEVEL"
	LOGFILE_ENV_VAR  = "ECS_LOGFILE"

	DEFAULT_LOGLEVEL = log15.LvlWarn
)

// This logger is the base that all other loggers should inherit from
var logger log15.Logger
var logfile string
var level log15.Lvl

// Initialize this logger once
var once sync.Once

func initLogger() {
	logger = log15.New()

	envLevel := os.Getenv(LOGLEVEL_ENV_VAR)
	SetLevel(envLevel)

	logfile = os.Getenv(LOGFILE_ENV_VAR)

	SetHandlerChain()
}

func init() {
	once.Do(initLogger)
}

// This package relies on the log15 backend. It is simply a place to centralize
// instantiations of loggers so they all have the same basic config
func SetLevel(logLevel string) {
	parsedLevel, err := log15.LvlFromString(strings.ToLower(logLevel))
	if err == nil {
		level = parsedLevel
	}

	SetHandlerChain()
}

func SetHandlerChain() {
	// Filehandler is an optional handler that always logs at debug level when
	// set, but only runs if the LOGFILE_ENV_VAR (ECS_LOGFILE) is set.
	var fileHandler log15.Handler
	if logfile == "" {
		fileHandler = log15.DiscardHandler()
	} else {
		fileHandler = log15.CallerStackHandler("%+v", log15.Must.FileHandler(logfile, log15.LogfmtFormat()))
	}

	logger.SetHandler(
		log15.LazyHandler(
			log15.MultiHandler(
				log15.LvlFilterHandler(level,
					log15.StdoutHandler,
				),
				fileHandler,
			),
		),
	)
}

func ForModule(module string) log15.Logger {
	once.Do(initLogger)

	SetHandlerChain()

	return logger.New("module", module)
}
