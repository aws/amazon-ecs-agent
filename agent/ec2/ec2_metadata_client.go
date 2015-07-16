// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

const (
	EC2_METADATA_SERVICE_URL                      = "http://169.254.169.254"
	SECURITY_CREDENTIALS_RESOURCE                 = "/2014-02-25/meta-data/iam/security-credentials/"
	INSTANCE_IDENTITY_DOCUMENT_RESOURCE           = "/2014-02-25/dynamic/instance-identity/document"
	INSTANCE_IDENTITY_DOCUMENT_SIGNATURE_RESOURCE = "/2014-02-25/dynamic/instance-identity/signature"
	SIGNED_INSTANCE_IDENTITY_DOCUMENT_RESOURCE    = "/2014-02-25/dynamic/instance-identity/pkcs7"
	EC2_METADATA_REQUEST_TIMEOUT                  = time.Duration(1 * time.Second)
)

const (
	metadataRetries            = 5
	metadataRetryMaxDelay      = 2 * time.Second
	metadataRetryStartDelay    = 250 * time.Millisecond
	metadataRetryDelayMultiple = 2
)

type RoleCredentials struct {
	Code            string    `json:"Code"`
	LastUpdated     time.Time `json:"LastUpdated"`
	Type            string    `json:"Type"`
	AccessKeyId     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}

type InstanceIdentityDocument struct {
	InstanceId       string  `json:"instanceId"`
	InstanceType     string  `json:"instanceType"`
	Region           string  `json:"region"`
	PrivateIp        *string `json:"privateIp"`
	AvailabilityZone string  `json:"availabilityZone"`
}

type HttpClient interface {
	Get(string) (*http.Response, error)
}

type EC2MetadataClient interface {
	DefaultCredentials() (*RoleCredentials, error)
	ReadResource(string) ([]byte, error)
	InstanceIdentityDocument() (*InstanceIdentityDocument, error)
}

type ec2MetadataClientImpl struct {
	client HttpClient
}

func NewEC2MetadataClient(httpClient HttpClient) EC2MetadataClient {
	if httpClient == nil {
		var lowTimeoutDial http.RoundTripper = &http.Transport{
			Dial: (&net.Dialer{
				Timeout: EC2_METADATA_REQUEST_TIMEOUT,
			}).Dial,
		}

		httpClient = &http.Client{Transport: lowTimeoutDial}
	}

	return &ec2MetadataClientImpl{client: httpClient}
}

func (c *ec2MetadataClientImpl) DefaultCredentials() (*RoleCredentials, error) {
	securityCredentialResp, err := c.ReadResource(SECURITY_CREDENTIALS_RESOURCE)
	if err != nil {
		return nil, err
	}

	securityCredentialList := strings.Split(strings.TrimSpace(string(securityCredentialResp)), "\n")
	if len(securityCredentialList) == 0 {
		return nil, errors.New("No security credentials in response")
	}

	defaultCredentialName := securityCredentialList[0]

	rawResp, err := c.ReadResource(SECURITY_CREDENTIALS_RESOURCE + defaultCredentialName)
	if err != nil {
		return nil, err
	}
	var credential RoleCredentials
	err = json.Unmarshal(rawResp, &credential)
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func (c *ec2MetadataClientImpl) InstanceIdentityDocument() (*InstanceIdentityDocument, error) {
	rawIidResp, err := c.ReadResource(INSTANCE_IDENTITY_DOCUMENT_RESOURCE)
	if err != nil {
		return nil, err
	}

	var iid InstanceIdentityDocument

	err = json.Unmarshal(rawIidResp, &iid)
	if err != nil {
		return nil, err
	}
	return &iid, nil
}

func (c *ec2MetadataClientImpl) ResourceServiceUrl(path string) string {
	// TODO, override EC2_METADATA_SERVICE_URL based on the environment
	return EC2_METADATA_SERVICE_URL + path
}

func (c *ec2MetadataClientImpl) ReadResource(path string) ([]byte, error) {
	endpoint := c.ResourceServiceUrl(path)

	var err error
	var resp *http.Response
	utils.RetryNWithBackoff(utils.NewSimpleBackoff(metadataRetryStartDelay, metadataRetryMaxDelay, metadataRetryDelayMultiple, 0.2), metadataRetries, func() error {
		resp, err = c.client.Get(endpoint)
		if err == nil && resp.StatusCode == 200 {
			return nil
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err == nil {
			seelog.Warnf("Error accessing the EC2 Metadata Service; non-200 response: %v", resp.StatusCode)
			return fmt.Errorf("Error contacting EC2 Metadata service; non-200 response: %v", resp.StatusCode)
		} else {
			seelog.Warnf("Error accessing the EC2 Metadata Service; retrying: %v", err)
			return err
		}
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(resp.Body)
}

// DefaultClient is the client used for package level methods.
var DefaultClient = NewEC2MetadataClient(nil)

// ReadResource reads a given path from the EC2 metadata service using the
// default client
func ReadResource(path string) ([]byte, error) {
	return DefaultClient.ReadResource(path)
}

// GetInstanceIdentityDocument returns an InstanceIdentityDocument read using
// the default client
func GetInstanceIdentityDocument() (*InstanceIdentityDocument, error) {
	return DefaultClient.InstanceIdentityDocument()
}

// DefaultCredentials returns the instance's default role read using the default
// client
func DefaultCredentials() (*RoleCredentials, error) {
	return DefaultClient.DefaultCredentials()
}
