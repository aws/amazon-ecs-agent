// +build setup

package auth

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// NOTE: modify constants_test to suit your needs first
func TestSetup(t *testing.T) {
	cfg := &aws.Config{
		Region:      aws.String(IntegRegion),
		Credentials: credentials.NewSharedCredentials("", IntegAWSProfile),
	}
	client := secretsmanager.New(session.Must(session.NewSession(cfg)))

	// dont't use this password, except maybe on luggage
	literalSecret := `{"username":"usr","password":"12345"}`

	in := &secretsmanager.CreateSecretInput{
		Name:         aws.String(IntegSecretsName),
		SecretString: aws.String(literalSecret),
	}

	_, err := client.CreateSecret(in)
	if err != nil {
		t.Error("It failed. IDK why: ", err)
	}
}
