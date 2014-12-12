package signable

import (
	"io"
	"net/url"
)

type Signable interface {
	GetHost() string
	GetHeader(string) ([]string, bool)
	SetHeader(string, string)
	ReqURL() *url.URL
	ReqBody() io.Reader
	SetReqBody(io.ReadCloser)
	ReqMethod() string
}
