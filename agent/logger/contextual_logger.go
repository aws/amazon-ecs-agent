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
	"path/filepath"
	"runtime"

	"github.com/cihub/seelog"
)

// Contextual is a logger that can have custom context added to it. Once
// SetContext is called, it will print log messages with the additional context
// appended. Before SetContext is called, it will print messages using the
// default agent logger.
type Contextual struct {
	log     seelog.LoggerInterface
	context map[string]string
}

// Debugf formats message according to format specifier
// and writes to log with level = Debug.
func (c *Contextual) Debugf(format string, params ...interface{}) {
	if c.log == nil {
		seelog.Debugf(format, params...)
	} else {
		c.log.Debugf(format, params...)
	}
}

// Infof formats message according to format specifier
// and writes to log with level = Info.
func (c *Contextual) Infof(format string, params ...interface{}) {
	if c.log == nil {
		seelog.Infof(format, params...)
	} else {
		c.log.Infof(format, params...)
	}
}

// Warnf formats message according to format specifier
// and writes to log with level = Warn.
func (c *Contextual) Warnf(format string, params ...interface{}) error {
	if c.log == nil {
		return seelog.Warnf(format, params...)
	} else {
		return c.log.Warnf(format, params...)
	}
}

// Errorf formats message according to format specifier
// and writes to log with level = Error.
func (c *Contextual) Errorf(format string, params ...interface{}) error {
	if c.log == nil {
		return seelog.Errorf(format, params...)
	} else {
		return c.log.Errorf(format, params...)
	}
}

// Debug formats message using the default formats for its operands
// and writes to log with level = Debug
func (c *Contextual) Debug(v ...interface{}) {
	if c.log == nil {
		seelog.Debug(v...)
	} else {
		c.log.Debug(v...)
	}
}

// Info formats message using the default formats for its operands
// and writes to log with level = Info
func (c *Contextual) Info(v ...interface{}) {
	if c.log == nil {
		seelog.Info(v...)
	} else {
		c.log.Info(v...)
	}
}

// Warn formats message using the default formats for its operands
// and writes to log with level = Warn
func (c *Contextual) Warn(v ...interface{}) error {
	if c.log == nil {
		return seelog.Warn(v...)
	} else {
		return c.log.Warn(v...)
	}
}

// Error formats message using the default formats for its operands
// and writes to log with level = Error
func (c *Contextual) Error(v ...interface{}) error {
	if c.log == nil {
		return seelog.Error(v...)
	} else {
		return c.log.Error(v...)
	}
}

func (c *Contextual) SetContext(context map[string]string) {
	if c.log == nil {
		c.log = InitLogger()
		_, f, _, _ := runtime.Caller(1)
		context["module"] = filepath.Base(f)
		c.log.SetContext(context)
	}
}
