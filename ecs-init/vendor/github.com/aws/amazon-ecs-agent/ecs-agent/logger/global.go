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
	"sync"

	"github.com/cihub/seelog"
)

var (
	loggerMux              = &sync.RWMutex{}
	globalStructuredLogger *structuredLogger
)

func init() {
	loggerMux.Lock()
	defer loggerMux.Unlock()
	globalStructuredLogger = newStructuredLogger(DEFAULT_OUTPUT_FORMAT)
}

func setGlobalLogger(logger seelog.LoggerInterface, logFormat string) {
	loggerMux.Lock()
	defer loggerMux.Unlock()
	seelog.ReplaceLogger(logger)
	globalStructuredLogger = newStructuredLogger(logFormat)
}

func getGlobalStructuredLogger() *structuredLogger {
	loggerMux.RLock()
	defer loggerMux.RUnlock()
	return globalStructuredLogger
}

func Trace(message string, fields ...Fields) {
	l := getGlobalStructuredLogger()
	l.Trace(message, fields...)
}

func Debug(message string, fields ...Fields) {
	l := getGlobalStructuredLogger()
	l.Debug(message, fields...)
}

func Info(message string, fields ...Fields) {
	l := getGlobalStructuredLogger()
	l.Info(message, fields...)
}

func Warn(message string, fields ...Fields) {
	l := getGlobalStructuredLogger()
	l.Warn(message, fields...)
}

func Error(message string, fields ...Fields) {
	l := getGlobalStructuredLogger()
	l.Error(message, fields...)
}

func Critical(message string, fields ...Fields) {
	l := getGlobalStructuredLogger()
	l.Critical(message, fields...)
}
