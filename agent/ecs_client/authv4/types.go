package authv4

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/sign"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/signable"
	"net/http"
)

type Signer interface {
	Sign(signable.Signable) error
}

// Implements Signer
type DefaultSigner struct {
	credentials.AWSCredentialProvider

	Region, Service string

	ExtraHeaders []string

	Signer *sign.Signer
}

// Signs an http.Request by mutating it
type HttpSigner interface {
	SignHttpRequest(*http.Request) error
}

// A signed round-tripper signs a request and subsequently returns a response.
type RoundTripperSigner http.RoundTripper

type DefaultRoundTripSigner struct {
	HttpSigner

	Transport http.RoundTripper
}
