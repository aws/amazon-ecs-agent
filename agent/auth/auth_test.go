// +build integ

package auth

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/stretchr/testify/assert"
)

// NOTE: modify constants_test to suit your needs first
func TestAuth(t *testing.T) {
	cfg := &aws.Config{
		Region:      aws.String(IntegRegion),
		Credentials: credentials.NewSharedCredentials("", IntegAWSProfile),
	}
	sess := session.Must(session.NewSession(cfg))
	provider := NewAuthProvider(sess, IntegRegion, IntegSecretsName)

	auth := provider.GetAuth()

	assert.Equal(t, "usr", auth.Username)
	assert.Equal(t, "12345", auth.Password)
}
