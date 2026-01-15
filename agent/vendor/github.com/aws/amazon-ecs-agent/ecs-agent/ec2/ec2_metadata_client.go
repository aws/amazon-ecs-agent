// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package ec2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/providers"
)

const (
	// There are three categories of instance metadata.
	// 1. Instance metadata properties. Accessed through GetMetadata(path).
	// 2. Dynamic data. Accessed through GetDynamicData(path).
	// 3. User data. Access through GetUserData().
	// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html

	// Paths for the instance metadata properties.
	SecurityCredentialsResource      = "iam/security-credentials/"
	MacResource                      = "mac"
	AllMacResource                   = "network/interfaces/macs"
	VPCIDResourceFormat              = "network/interfaces/macs/%s/vpc-id"
	SubnetIDResourceFormat           = "network/interfaces/macs/%s/subnet-id"
	SpotInstanceActionResource       = "spot/instance-action"
	InstanceIDResource               = "instance-id"
	RegionResource                   = "placement/region"
	AvailabilityZoneID               = "placement/availability-zone-id"
	PrivateIPv4Resource              = "local-ipv4"
	PublicIPv4Resource               = "public-ipv4"
	IPv6Resource                     = "ipv6"
	OutpostARN                       = "outpost-arn"
	PrimaryIPV4VPCCIDRResourceFormat = "network/interfaces/macs/%s/vpc-ipv4-cidr-block"
	TargetLifecycleState             = "autoscaling/target-lifecycle-state"

	// Paths for dynamic data categories.
	InstanceIdentityDocumentResource          = "instance-identity/document"
	InstanceIdentityDocumentSignatureResource = "instance-identity/signature"
)

const (
	metadataRetries = 5
)

// RoleCredentials contains the information associated with an IAM role
type RoleCredentials struct {
	Code            string    `json:"Code"`
	LastUpdated     time.Time `json:"LastUpdated"`
	Type            string    `json:"Type"`
	AccessKeyId     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}

type HttpClient interface {
	GetMetadata(context.Context, *imds.GetMetadataInput, ...func(*imds.Options)) (*imds.GetMetadataOutput, error)
	GetDynamicData(context.Context, *imds.GetDynamicDataInput, ...func(*imds.Options)) (*imds.GetDynamicDataOutput, error)
	GetInstanceIdentityDocument(context.Context, *imds.GetInstanceIdentityDocumentInput, ...func(*imds.Options)) (*imds.GetInstanceIdentityDocumentOutput, error)
	GetUserData(context.Context, *imds.GetUserDataInput, ...func(*imds.Options)) (*imds.GetUserDataOutput, error)
	GetRegion(context.Context, *imds.GetRegionInput, ...func(*imds.Options)) (*imds.GetRegionOutput, error)
}

// EC2MetadataClient is the client used to get metadata from instance metadata service
type EC2MetadataClient interface {
	DefaultCredentials() (*RoleCredentials, error)
	GetMetadata(string) (string, error)
	GetDynamicData(string) (string, error)
	InstanceIdentityDocument() (imds.InstanceIdentityDocument, error)
	VPCID(mac string) (string, error)
	SubnetID(mac string) (string, error)
	PrimaryENIMAC() (string, error)
	AllENIMacs() (string, error)
	InstanceID() (string, error)
	GetUserData() (string, error)
	Region() (string, error)
	AvailabilityZoneID() (string, error)
	PrivateIPv4Address() (string, error)
	PublicIPv4Address() (string, error)
	IPv6Address() (string, error)
	SpotInstanceAction() (string, error)
	OutpostARN() (string, error)
	TargetLifecycleState() (string, error)
}

// ec2MetadataClientImpl implements EC2MetadataClient and cache data for following IMDS requests.
// PrimaryENIMAC()
// VPCID(mac)
// SubnetID(mac)
// InstanceID()
// GetUserData()
// Region()
// AvailabilityZoneID()
// InstanceIdentityDocument()
type ec2MetadataClientImpl struct {
	client                     HttpClient
	cache                      map[string]string
	cacheMu                    sync.RWMutex
	instanceIdentityDocCache   *imds.InstanceIdentityDocument
	instanceIdentityDocCacheMu sync.RWMutex
}

// NewEC2MetadataClient creates an ec2metadata client to retrieve metadata.
// Pass a non-nil HttpClient to mock behavior in tests.
func NewEC2MetadataClient(client HttpClient) (EC2MetadataClient, error) {
	if client == nil {
		credentialsProvider := providers.NewInstanceCredentialsCache(
			false,
			providers.NewRotatingSharedCredentialsProviderV2(),
			nil,
		)
		cfg, err := config.LoadDefaultConfig(
			context.TODO(),
			config.WithCredentialsProvider(credentialsProvider),
			config.WithRetryMaxAttempts(metadataRetries),
		)
		if err != nil {
			return nil, err
		}

		return &ec2MetadataClientImpl{
			client: imds.NewFromConfig(cfg),
			cache:  make(map[string]string),
		}, nil
	} else {
		return &ec2MetadataClientImpl{
			client: client,
			cache:  make(map[string]string),
		}, nil
	}
}

// DefaultCredentials returns the credentials associated with the instance iam role
func (c *ec2MetadataClientImpl) DefaultCredentials() (*RoleCredentials, error) {
	securityCredential, err := c.GetMetadata(SecurityCredentialsResource)
	if err != nil {
		return nil, err
	}

	securityCredentialList := strings.Split(strings.TrimSpace(securityCredential), "\n")
	if len(securityCredentialList) == 0 {
		return nil, errors.New("No security credentials in response")
	}

	defaultCredentialName := securityCredentialList[0]

	defaultCredentialStr, err := c.GetMetadata(SecurityCredentialsResource + defaultCredentialName)
	if err != nil {
		return nil, err
	}
	var credential RoleCredentials
	err = json.Unmarshal([]byte(defaultCredentialStr), &credential)
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

// GetDynamicData returns the dynamic data with provided path from instance metadata
func (c *ec2MetadataClientImpl) GetDynamicData(path string) (string, error) {
	output, err := c.client.GetDynamicData(context.TODO(), &imds.GetDynamicDataInput{
		Path: path,
	})
	if err != nil {
		return "", err
	}

	content, err := io.ReadAll(output.Content)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// InstanceIdentityDocument returns instance identity documents
// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-identity-documents.html
func (c *ec2MetadataClientImpl) InstanceIdentityDocument() (imds.InstanceIdentityDocument, error) {
	c.instanceIdentityDocCacheMu.RLock()
	if c.instanceIdentityDocCache != nil {
		doc := *c.instanceIdentityDocCache
		c.instanceIdentityDocCacheMu.RUnlock()
		return doc, nil
	}
	c.instanceIdentityDocCacheMu.RUnlock()

	output, err := c.client.GetInstanceIdentityDocument(context.TODO(), &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return imds.InstanceIdentityDocument{}, err
	}

	c.instanceIdentityDocCacheMu.Lock()
	c.instanceIdentityDocCache = &output.InstanceIdentityDocument
	c.instanceIdentityDocCacheMu.Unlock()

	return output.InstanceIdentityDocument, nil
}

// GetMetadata returns the metadata from instance metadata service specified by the path
func (c *ec2MetadataClientImpl) GetMetadata(path string) (string, error) {
	output, err := c.client.GetMetadata(context.TODO(), &imds.GetMetadataInput{
		Path: path,
	})
	if err != nil {
		return "", err
	}

	content, err := io.ReadAll(output.Content)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// PrimaryENIMAC returns the MAC address for the primary
// network interface of the instance
func (c *ec2MetadataClientImpl) PrimaryENIMAC() (string, error) {
	return c.getCachedMetadata(MacResource, func() (string, error) {
		return c.GetMetadata(MacResource)
	})
}

// AllENIMacs returns the mac addresses for all the network interfaces attached to the instance
func (c *ec2MetadataClientImpl) AllENIMacs() (string, error) {
	return c.GetMetadata(AllMacResource)
}

// VPCID returns the VPC id for the network interface, given
// its mac address
func (c *ec2MetadataClientImpl) VPCID(mac string) (string, error) {
	path := fmt.Sprintf(VPCIDResourceFormat, mac)
	return c.getCachedMetadata(path, func() (string, error) {
		return c.GetMetadata(path)
	})
}

// SubnetID returns the subnet id for the network interface,
// given its mac address
func (c *ec2MetadataClientImpl) SubnetID(mac string) (string, error) {
	path := fmt.Sprintf(SubnetIDResourceFormat, mac)
	return c.getCachedMetadata(path, func() (string, error) {
		return c.GetMetadata(path)
	})
}

// InstanceID returns the id of this instance.
func (c *ec2MetadataClientImpl) InstanceID() (string, error) {
	return c.getCachedMetadata(InstanceIDResource, func() (string, error) {
		return c.GetMetadata(InstanceIDResource)
	})
}

// GetUserData returns the userdata specified when launch your instance.
func (c *ec2MetadataClientImpl) GetUserData() (string, error) {
	return c.getCachedMetadata("user-data", func() (string, error) {
		output, err := c.client.GetUserData(context.TODO(), &imds.GetUserDataInput{})
		if err != nil {
			return "", err
		}

		content, err := io.ReadAll(output.Content)
		if err != nil {
			return "", err
		}

		return string(content), nil
	})
}

// Region returns the region the instance is running in.
func (c *ec2MetadataClientImpl) Region() (string, error) {
	return c.getCachedMetadata(RegionResource, func() (string, error) {
		return c.GetMetadata(RegionResource)
	})
}

// AvailabilityZoneID returns the availability zone ID that the instance is running in.
func (c *ec2MetadataClientImpl) AvailabilityZoneID() (string, error) {
	return c.getCachedMetadata(AvailabilityZoneID, func() (string, error) {
		return c.GetMetadata(AvailabilityZoneID)
	})
}

// PublicIPv4Address returns the public IPv4 of this instance
// if this instance has a public address
func (c *ec2MetadataClientImpl) PublicIPv4Address() (string, error) {
	return c.GetMetadata(PublicIPv4Resource)
}

// PrivateIPv4Address returns the private IPv4 of this instance
func (c *ec2MetadataClientImpl) PrivateIPv4Address() (string, error) {
	return c.GetMetadata(PrivateIPv4Resource)
}

// IPv6Address returns the IPv6 address of this instance
func (c *ec2MetadataClientImpl) IPv6Address() (string, error) {
	return c.GetMetadata(IPv6Resource)
}

// SpotInstanceAction returns the spot instance-action, if it has been set.
// If the time has not been set (ie, the instance is not scheduled for interruption)
// then this function returns an error.
// see https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-interruptions.html#using-spot-instances-managing-interruptions
func (c *ec2MetadataClientImpl) SpotInstanceAction() (string, error) {
	return c.GetMetadata(SpotInstanceActionResource)
}

func (c *ec2MetadataClientImpl) OutpostARN() (string, error) {
	return c.GetMetadata(OutpostARN)
}

func (c *ec2MetadataClientImpl) TargetLifecycleState() (string, error) {
	return c.GetMetadata(TargetLifecycleState)
}

// getCachedMetadata is a helper that implements the cache-aside pattern for metadata values.
// It checks the cache first, and if not found, calls the fetch function and stores the result.
func (c *ec2MetadataClientImpl) getCachedMetadata(key string, fetch func() (string, error)) (string, error) {
	c.cacheMu.RLock()
	v, ok := c.cache[key]
	c.cacheMu.RUnlock()

	if ok {
		return v, nil
	}

	v, err := fetch()
	if err != nil {
		return "", err
	}

	c.cacheMu.Lock()
	c.cache[key] = v
	c.cacheMu.Unlock()

	return v, nil
}
