package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
)

type InstanceCredentialsCache struct {
	providers []aws.CredentialsProvider
}

<<<<<<< HEAD
func (p *InstanceCredentialsCache) Retrieve(ctx context.Context) (aws.Credentials, error) {
=======
func (p *InstanceCredentialsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
>>>>>>> 8ad27e1790 (clean up InstanceCredentialsProvider)
	var errs []error
	for _, provider := range p.providers {
		creds, err := provider.Retrieve(ctx)
		if creds.HasKeys() && err == nil {
			logger.Info(fmt.Sprintf("Successfully got ECS instance credentials from provider: %s", creds.Source))
			return creds, nil
		}

		errs = append(errs, err)
	}

	err := fmt.Errorf("no valid providers in chain: %s", errors.Join(errs...))
	logger.Error(fmt.Sprintf("Error getting ECS instance credentials from credentials chain: %s", err))
	return aws.Credentials{}, err
}

func defaultCreds(options func(*ec2rolecreds.Options)) aws.CredentialsProviderFunc {
	return func(ctx context.Context) (aws.Credentials, error) {
		cfg, err := config.LoadDefaultConfig(ctx, config.WithEC2RoleCredentialOptions(options))
		if err != nil {
			return aws.Credentials{}, err
		}

		return cfg.Credentials.Retrieve(ctx)

	}
}
