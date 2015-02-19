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

package signable

import (
	"io"
	"net/http"
	"net/url"
)

type HttpRequest struct{ *http.Request }

func (hr HttpRequest) ReqBody() io.Reader {
	return hr.Body
}

func (hr HttpRequest) SetReqBody(r io.ReadCloser) {
	hr.Body = r
}

func (hr HttpRequest) GetHost() string {
	if hr.Host != "" {
		return hr.Host
	}
	if hr.URL.Host != "" {
		return hr.URL.Host
	}
	return hr.Header.Get("host")
}

func (hr HttpRequest) GetHeader(name string) ([]string, bool) {
	headers, present := hr.Header[name]
	return headers, present
}

func (hr HttpRequest) SetHeader(name, value string) {
	hr.Header.Set(name, value)
}

func (hr HttpRequest) ReqURL() *url.URL {
	return hr.URL
}

func (hr HttpRequest) ReqMethod() string {
	return hr.Method
}
