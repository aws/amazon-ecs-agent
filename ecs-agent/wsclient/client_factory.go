package wsclient

import (
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
)

// ClientFactory interface abstracts the method that creates new ClientServer
// objects. This is helpful when writing unit tests.
type ClientFactory interface {
	New(url string, credentialProvider *credentials.Credentials, rwTimeout time.Duration, cfg *WSClientMinAgentConfig) ClientServer
}
