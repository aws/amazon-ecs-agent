package credentials

import "errors"

type AWSCredentials struct {
	AccessKey string
	SecretKey string
	Token     string
}

func (creds *AWSCredentials) Credentials() (*AWSCredentials, error) {
	if creds == nil {
		return nil, errors.New("Credentials not set")
	}
	return creds, nil
}

type AWSCredentialProvider interface {
	Credentials() (*AWSCredentials, error)
}

type RefreshableAWSCredentialProvider interface {
	AWSCredentialProvider

	Refresh()

	NeedsRefresh() bool
}

type NoCredentialProviderError struct{}

func (ncp NoCredentialProviderError) Error() string {
	return "No credential provider was set"
}
