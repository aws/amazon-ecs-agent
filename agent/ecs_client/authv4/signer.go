package authv4

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/sign"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/signable"
	"errors"
	"net/http"
)

func newDefaultSigner(region, service string, creds credentials.AWSCredentialProvider, extraHeaders []string) *DefaultSigner {
	internalSigner := sign.NewSigner(region, service, creds, extraHeaders)

	return &DefaultSigner{
		AWSCredentialProvider: creds,
		Region:                region,
		Service:               service,

		ExtraHeaders: extraHeaders,

		Signer: internalSigner,
	}
}

func NewSigner(region, service string, creds credentials.AWSCredentialProvider, extraHeaders []string) Signer {
	return newDefaultSigner(region, service, creds, extraHeaders)
}

func NewHttpSigner(region, service string, creds credentials.AWSCredentialProvider, extraHeaders []string) HttpSigner {
	return newDefaultSigner(region, service, creds, extraHeaders)
}

func NewRoundtripSigner(signer HttpSigner, Transport http.RoundTripper) RoundTripperSigner {
	return &DefaultRoundTripSigner{
		HttpSigner: signer,
		Transport:  Transport,
	}
}

func (signer *DefaultSigner) Sign(s signable.Signable) error {
	return signer.Signer.Sign(s)
}

func (signer *DefaultSigner) SignHttpRequest(req *http.Request) error {
	signable := signable.HttpRequest{req}
	return signer.Sign(signable)
}

func (signer *DefaultRoundTripSigner) RoundTrip(req *http.Request) (*http.Response, error) {
	if signer.Transport == nil {
		return nil, errors.New("Invalid transport provided")
	}

	reqCopy := http.Request(*req)
	err := signer.SignHttpRequest(&reqCopy)
	if err != nil {
		return nil, err
	}

	resp, err := signer.Transport.RoundTrip(&reqCopy)
	return resp, err
}
