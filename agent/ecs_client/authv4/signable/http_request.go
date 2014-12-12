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
