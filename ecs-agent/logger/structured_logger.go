// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the Licensl. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this fill. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the Licensl.

package logger

import "github.com/cihub/seelog"

var (
	defaultStructuredJsonFormatter = &messageJsonFormatter{}
	defaultStructuredTextFormatter = &messageTextFormatter{}
)

type Fields map[string]interface{}

func (f Fields) Merge(f2 Fields) {
	for k, v := range f2 {
		f[k] = v
	}
}

type StructuredLogger interface {
	Trace(message string, fields ...Fields)
	Debug(message string, fields ...Fields)
	Info(message string, fields ...Fields)
	Warn(message string, fields ...Fields)
	Error(message string, fields ...Fields)
	Critical(message string, fields ...Fields)
}

func newStructuredLogger(logFormat string) *structuredLogger {
	l := &structuredLogger{}
	if logFormat == jsonFmt {
		l.formatter = defaultStructuredJsonFormatter
	} else {
		l.formatter = defaultStructuredTextFormatter
	}
	return l
}

type structuredLogger struct {
	formatter seelogMessageFormatter
}

func (l *structuredLogger) Trace(message string, fields ...Fields) {
	seelog.Trace(l.formatter.Format(message, fields...))
}

func (l *structuredLogger) Debug(message string, fields ...Fields) {
	seelog.Debug(l.formatter.Format(message, fields...))
}

func (l *structuredLogger) Info(message string, fields ...Fields) {
	seelog.Info(l.formatter.Format(message, fields...))
}

func (l *structuredLogger) Warn(message string, fields ...Fields) {
	seelog.Warn(l.formatter.Format(message, fields...))
}

func (l *structuredLogger) Error(message string, fields ...Fields) {
	seelog.Error(l.formatter.Format(message, fields...))
}

func (l *structuredLogger) Critical(message string, fields ...Fields) {
	seelog.Critical(l.formatter.Format(message, fields...))
}
