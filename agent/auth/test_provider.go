package auth

import . "github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"

type TestCredentialProvider struct{}

func (t TestCredentialProvider) Credentials() (*AWSCredentials, error) {
	return &AWSCredentials{
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Token:     "",
	}, nil
}
