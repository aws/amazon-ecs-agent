package credentials

import (
	"errors"
	"os"
)

type EnvironmentCredentialProvider struct{}

func NewEnvironmentCredentialProvider() *EnvironmentCredentialProvider {
	return new(EnvironmentCredentialProvider)
}

// Credentials pulls the credentials from common environment variables.
func (ecp EnvironmentCredentialProvider) Credentials() (*AWSCredentials, error) {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if len(accessKey) > 0 && len(secretKey) > 0 {
		if len(sessionToken) > 0 {
			return &AWSCredentials{accessKey, secretKey, sessionToken}, nil
		}
		return &AWSCredentials{AccessKey: accessKey, SecretKey: secretKey}, nil
	}
	return nil, errors.New("Unable to find credentials in the environment")
}
