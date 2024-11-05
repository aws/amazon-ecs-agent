package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type InstanceCredentialsProvider struct {
	providers []aws.CredentialsProvider
}

func (p *InstanceCredentialsProvider) Retrieve(ctx context.Context) (creds aws.Credentials, err error) {
	defer func() {
		if err != nil {
			logger.Error(fmt.Sprintf("Error getting ECS instance credentials from credentials chain: %s", err))
		} else {
			logger.Info(fmt.Sprintf("Successfully got ECS instance credentials from provider: %s", creds.Source))
		}
	}()

	var errs []error
	for _, provider := range p.providers {
		creds, err := provider.Retrieve(ctx)
		if err == nil {
			return creds, nil
		}

		errs = append(errs, err)
	}

	return aws.Credentials{}, fmt.Errorf("no valid providers in chain: %s", errors.Join(errs...))
}
