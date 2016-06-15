// Copyright (c) 2012 - Cloud Instruments Co., Ltd.
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package seelog

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var workingDir = ""

func init() {
	setWorkDir()
}

func setWorkDir() {
	workDir, workingDirError := os.Getwd()
	if workingDirError != nil {
		workingDir = string(os.PathSeparator)
		return
	}

	workingDir = workDir + string(os.PathSeparator)
}

// Represents runtime caller context
type LogContextInterface interface {
	// Caller func name
	Func() string
	// Caller line num
	Line() int
	// Caller file short path
	ShortPath() string
	// Caller file full path
	FullPath() string
	// Caller file name (without path)
	FileName() string
	// True if the context is correct and may be used.
	// If false, then an error in context evaluation occurred and
	// all its other data may be corrupted.
	IsValid() bool
	// Time when log func was called
	CallTime() time.Time
}

// Returns context of the caller
func currentContext() (LogContextInterface, error) {
	return specificContext(1)
}

func extractCallerInfo(skip int) (fullPath string, shortPath string, funcName string, lineNumber int, err error) {
	pc, fullPath, line, ok := runtime.Caller(skip)

	if !ok {
		return "", "", "", 0, errors.New("error during runtime.Caller")
	}

	//TODO:Currently fixes bug in weekly.2012-03-13+: Caller returns incorrect separators
	//Delete later

	fullPath = strings.Replace(fullPath, "\\", string(os.PathSeparator), -1)
	fullPath = strings.Replace(fullPath, "/", string(os.PathSeparator), -1)

	if strings.HasPrefix(fullPath, workingDir) {
		shortPath = fullPath[len(workingDir):]
	} else {
		shortPath = fullPath
	}

	funName := runtime.FuncForPC(pc).Name()
	var functionName string
	if strings.HasPrefix(funName, workingDir) {
		functionName = funName[len(workingDir):]
	} else {
		functionName = funName
	}

	return fullPath, shortPath, functionName, line, nil
}

// Returns context of the function with placed "skip" stack frames of the caller
// If skip == 0 then behaves like currentContext
// Context is returned in any situation, even if error occurs. But, if an error
// occurs, the returned context is an error context, which contains no paths
// or names, but states that they can't be extracted.
func specificContext(skip int) (LogContextInterface, error) {
	callTime := time.Now()

	if skip < 0 {
		negativeStackFrameErr := errors.New("can not skip negative stack frames")
		return &errorContext{callTime, negativeStackFrameErr}, negativeStackFrameErr
	}

	fullPath, shortPath, function, line, err := extractCallerInfo(skip + 2)
	if err != nil {
		return &errorContext{callTime, err}, err
	}
	_, fileName := filepath.Split(fullPath)
	return &logContext{function, line, shortPath, fullPath, fileName, callTime}, nil
}

// Represents a normal runtime caller context
type logContext struct {
	funcName  string
	line      int
	shortPath string
	fullPath  string
	fileName  string
	callTime  time.Time
}

func (context *logContext) IsValid() bool {
	return true
}

func (context *logContext) Func() string {
	return context.funcName
}

func (context *logContext) Line() int {
	return context.line
}

func (context *logContext) ShortPath() string {
	return context.shortPath
}

func (context *logContext) FullPath() string {
	return context.fullPath
}

func (context *logContext) FileName() string {
	return context.fileName
}

func (context *logContext) CallTime() time.Time {
	return context.callTime
}

const (
	errorContextFunc      = "Func() error:"
	errorContextShortPath = "ShortPath() error:"
	errorContextFullPath  = "FullPath() error:"
	errorContextFileName  = "FileName() error:"
)

// Represents an error context
type errorContext struct {
	errorTime time.Time
	err       error
}

func (errContext *errorContext) IsValid() bool {
	return false
}

func (errContext *errorContext) Line() int {
	return -1
}

func (errContext *errorContext) Func() string {
	return errorContextFunc + errContext.err.Error()
}

func (errContext *errorContext) ShortPath() string {
	return errorContextShortPath + errContext.err.Error()
}

func (errContext *errorContext) FullPath() string {
	return errorContextFullPath + errContext.err.Error()
}

func (errContext *errorContext) FileName() string {
	return errorContextFileName + errContext.err.Error()
}

func (errContext *errorContext) CallTime() time.Time {
	return errContext.errorTime
}
