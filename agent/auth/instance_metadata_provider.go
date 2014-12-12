package auth

import (
	"errors"
	"sync"
	"time"

	. "github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
)

const (
	TOKEN_REFRESH_FREQUENCY = 60.00 // Minutes

	// How many minutes early to refresh expiring tokens
	TOKEN_EXPIRATION_REFRESH_MINUTES time.Duration = 15.00 * time.Minute
)

type InstanceMetadataCredentialProvider struct {
	expiration     *time.Time
	lastCheck      *time.Time
	metadataClient *ec2.EC2MetadataClient
	credentials    AWSCredentials
	lock           sync.Mutex
}

func NewInstanceMetadataCredentialProvider() *InstanceMetadataCredentialProvider {
	imcp := new(InstanceMetadataCredentialProvider)

	imcp.metadataClient = ec2.NewEC2MetadataClient()
	return imcp
}

func (imcp *InstanceMetadataCredentialProvider) shouldRefresh() bool {
	if imcp.expiration != nil {
		if time.Now().Add(TOKEN_EXPIRATION_REFRESH_MINUTES).After(*imcp.expiration) {
			// Within the expiration window
			return true
		}
		// They'll expire, but haven't yet
		return false
	}

	if imcp.lastCheck == nil ||
		time.Since(*imcp.lastCheck).Minutes() > TOKEN_REFRESH_FREQUENCY {

		return true
	}
	return false
}

func (imcp *InstanceMetadataCredentialProvider) Credentials() (*AWSCredentials, error) {

	imcp.lock.Lock()
	defer imcp.lock.Unlock()

	if imcp.shouldRefresh() {
		now := time.Now() // can't &time.Now()
		imcp.lastCheck = &now
		credStruct, err := imcp.metadataClient.DefaultCredentials()
		if err != nil {
			return &imcp.credentials, err
		}
		imcp.credentials = AWSCredentials{
			AccessKey: credStruct.AccessKeyId,
			SecretKey: credStruct.SecretAccessKey,
			Token:     credStruct.Token,
		}
		imcp.expiration = &credStruct.Expiration
	}

	if len(imcp.credentials.AccessKey) == 0 || len(imcp.credentials.SecretKey) == 0 {
		return nil, errors.New("Unable to find credentials in the instance metadata service")
	}

	return &imcp.credentials, nil
}
