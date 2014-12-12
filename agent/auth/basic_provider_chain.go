package auth

import "errors"
import . "github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"

type BasicAWSCredentialProvider struct {
	providers           []AWSCredentialProvider
	lastUsedProviderNdx int
}

func newBasicAWSCredentialProvider() *BasicAWSCredentialProvider {
	provider := new(BasicAWSCredentialProvider)
	provider.lastUsedProviderNdx = -1
	return provider
}

// NewBasicAWSCredentialProvider creates a credential provider chain with
// commonly desired defaults. It pulls credentials from the environment and then
// the instance metadata
// TODO: Add the aws-cli's saved credentials format which reads from a file
func NewBasicAWSCredentialProvider() *BasicAWSCredentialProvider {
	provider := newBasicAWSCredentialProvider()

	provider.AddProvider(NewInstanceMetadataCredentialProvider())
	provider.AddProvider(NewEnvironmentCredentialProvider())

	return provider
}

// AddProvider allows a custom provider to be added. Calling this will add the
// provider as the first in order of preference for usage
func (bcp *BasicAWSCredentialProvider) AddProvider(acp AWSCredentialProvider) {
	// Prepend credentials to prefer later added ones
	bcp.providers = append([]AWSCredentialProvider{acp}, bcp.providers...)
}

// Credentials returns valid AWSCredentials from a provider. It "sticks" to the
// last known good provider for as long as it works and will automatically
// refresh credentials if needed
func (bcp BasicAWSCredentialProvider) Credentials() (*AWSCredentials, error) {
	if bcp.lastUsedProviderNdx >= 0 {
		return bcp.providers[bcp.lastUsedProviderNdx].Credentials()
	}

	for ndx, provider := range bcp.providers {
		credentials, err := provider.Credentials()
		if err == nil {
			bcp.lastUsedProviderNdx = ndx
			return credentials, err
		}
	}

	return nil, errors.New("Unable to find any working credential provider")
}
