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
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cihub/seelog"
)

// This will be used as a trick for seelog formatter to identify messages formatted by our seelogMessageFormatter
const (
	structuredTxtFormatPrefix  = "logger=structured "
	structuredJsonFormatPrefix = `"logger":"structured",`
)

type seelogMessageFormatter interface {
	Format(message string, fields ...Fields) string
}

type messageJsonFormatter struct {
}

func (f *messageJsonFormatter) Format(message string, fields ...Fields) string {
	var fieldsBuf *bytes.Buffer
	if len(fields) > 0 {
		fieldsBuf = bufferPool.Get()
		defer bufferPool.Put(fieldsBuf)
		enc := json.NewEncoder(fieldsBuf)
		fieldsMap := make(map[string]interface{})
		for _, fi := range fields {
			for k, v := range fi {
				if vErr, ok := v.(error); ok {
					fieldsMap[k] = vErr.Error()
				} else {
					fieldsMap[k] = v
				}
			}
		}
		if err := enc.Encode(fieldsMap); err != nil {
			// fallback to default message in the event of any error
			seelog.Debug(fmt.Errorf("failed to marshal message fields to JSON, %v", err))
		} else {
			// Removing the trailing line break (https://golang.org/pkg/encoding/json/#Encoder.Encode) and the closing bracket
			fieldsBuf.Truncate(fieldsBuf.Len() - 2)
		}
	}

	msgBuf := bufferPool.Get()
	defer bufferPool.Put(msgBuf)
	msgBuf.WriteString(structuredJsonFormatPrefix)
	msgBuf.WriteString(`"msg":`)
	msgBuf.WriteString(fmt.Sprintf("%q", message))
	if fieldsBuf != nil && fieldsBuf.Len() > 0 {
		msgBuf.WriteByte(',')
		msgBuf.Write(fieldsBuf.Bytes()[1:])
	}
	return msgBuf.String()
}

type messageTextFormatter struct {
}

func (f *messageTextFormatter) Format(message string, fields ...Fields) string {
	buf := bufferPool.Get()
	defer bufferPool.Put(buf)
	buf.WriteString(structuredTxtFormatPrefix)
	buf.WriteString("msg=")
	buf.WriteString(fmt.Sprintf("%q", message))
	for _, fi := range fields {
		for k, v := range fi {
			f.appendKeyValue(buf, k, v)
		}
	}
	return buf.String()
}

func (f *messageTextFormatter) appendKeyValue(buf *bytes.Buffer, key string, value interface{}) {
	if buf.Len() > 0 {
		buf.WriteByte(' ')
	}
	buf.WriteString(key)
	buf.WriteByte('=')

	var strVal string
	switch t := value.(type) {
	case string:
		strVal = fmt.Sprintf("%q", t)
	case error:
		strVal = fmt.Sprintf("%q", t.Error())
	default:
		strVal = fmt.Sprintf("%+v", t)
	}

	buf.WriteString(strVal)
}
