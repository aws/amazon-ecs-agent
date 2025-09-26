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

type InstanceCredentialsProvider struct {
	providers []aws.CredentialsProvider
}

func (p *InstanceCredentialsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
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

// NewInstanceCredentialsCache returns a chain of instance credentials providers wrapped in a credentials cache.
// The instance credentials chain is the default credentials chain plus the "rotating shared credentials provider",
// so credentials will be checked in this order:
//
//  1. Env vars (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY).
//
//  2. Shared credentials file (https://docs.aws.amazon.com/ses/latest/DeveloperGuide/create-shared-credentials-file.html) (file at ~/.aws/credentials containing access key id and secret access key).
//
//  3. EC2 role credentials. This is an IAM role that the user specifies when they launch their EC2 container instance (ie ecsInstanceRole (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/instance_IAM_role.html)).
//
//  4. Rotating shared credentials file located at /rotatingcreds/credentials
//
//     The default credential chain provided by the SDK includes:
//     * EnvProvider
//     * SharedCredentialsProvider
//     * RemoteCredProvider (EC2RoleProvider)
//
//     In the case of ECS-A, there are a couple considerations:
//
//     * The `SharedCredentialsProvider` takes
//     precedence over the `RotatingSharedCredentialsProvider` and this results
//     in the credentials not being refreshed. To mitigate this issue, we will
//     reorder the credential chain and ensure that `RotatingSharedCredentialsProvider`
//     takes precedence over the `SharedCredentialsProvider` for ECS-A.
//
//     * On EC2, the `EC2RoleProvider` takes precedence over the `RotatingSharedCredentialsProvider`.
//     Prioritizing `RotatingSharedCredentialsProvider` over the
//     `EC2RoleProvider` ensures that SSM credentials will be used if they are available,
//     and the EC2 credentials will only be used as a last-resort.

func NewInstanceCredentialsCache(
	isExternal bool,
	rotatingSharedCreds aws.CredentialsProvider,
	imdsClient ec2rolecreds.GetMetadataAPIClient,
) *aws.CredentialsCache {
	var providers []aws.CredentialsProvider

	// If imdsClient is nil, the SDK will default to the EC2 IMDS client.
	// Pass a non-nil imdsClient to stub it out in tests.
	options := func(o *ec2rolecreds.Options) {
		o.Client = imdsClient
	}

	if isExternal {
		providers = []aws.CredentialsProvider{
			envCreds,
			rotatingSharedCreds,
			sharedCreds,
			ec2rolecreds.New(options),
		}
	} else {
		providers = []aws.CredentialsProvider{
			defaultCreds(options),
			rotatingSharedCreds,
		}
	}

	return aws.NewCredentialsCache(
		&InstanceCredentialsProvider{
			providers: providers,
		},
	)
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

var envCreds = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
	cfg, err := config.NewEnvConfig()
	return cfg.Credentials, err
})

var sharedCreds = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
	// Load the env config to get shared config values from env vars (AWS_PROFILE and AWS_SHARED_CREDENTIALS_FILE).
	envCfg, err := config.NewEnvConfig()
	if err != nil {
		return aws.Credentials{}, err
	}

	// If shared config env vars are unset, use the default values.
	if envCfg.SharedConfigProfile == "" {
		envCfg.SharedConfigProfile = config.DefaultSharedConfigProfile
	}
	if envCfg.SharedCredentialsFile == "" {
		envCfg.SharedCredentialsFile = config.DefaultSharedCredentialsFilename()
	}

	cfg, err := config.LoadSharedConfigProfile(ctx, envCfg.SharedConfigProfile, func(option *config.LoadSharedConfigOptions) {
		option.CredentialsFiles = []string{envCfg.SharedCredentialsFile}
	})
	return cfg.Credentials, err
})
