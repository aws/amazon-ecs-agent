// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"io/ioutil"
	"net"
	net_http "net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ec2/http"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	EC2MetadataServiceURL                     = "http://169.254.169.254"
	SecurityCrednetialsResource               = "/2014-02-25/meta-data/iam/security-credentials/"
	InstanceIdentityDocumentResource          = "/2014-02-25/dynamic/instance-identity/document"
	InstanceIdentityDocumentSignatureResource = "/2014-02-25/dynamic/instance-identity/signature"
	SignedInstanceIdentityDocumentResource    = "/2014-02-25/dynamic/instance-identity/pkcs7"

	macResource            = "/2014-02-25/meta-data/mac"
	vpcIDResourceFormat    = "/2014-02-25/meta-data/network/interfaces/macs/%s/vpc-id"
	subnetIDResourceFormat = "/2014-02-25/meta-data/network/interfaces/macs/%s/subnet-id"

	EC2MetadataRequestTimeout = 1 * time.Second
)

const (
	metadataRetries            = 5
	metadataRetryMaxDelay      = 2 * time.Second
	metadataRetryStartDelay    = 250 * time.Millisecond
	metadataRetryDelayMultiple = 2
)

// InstanceIdentityDocument stores the fields that constitute the identity
// of the EC2 Instance where the agent is running
type InstanceIdentityDocument struct {
	InstanceId       string  `json:"instanceId"`
	InstanceType     string  `json:"instanceType"`
	Region           string  `json:"region"`
	PrivateIp        *string `json:"privateIp"`
	AvailabilityZone string  `json:"availabilityZone"`
}

// EC2MetadataClient is the EC2 Metadata Service Client used by the
// ECS Agent
type EC2MetadataClient interface {
	// ReadResource reads the metadata associated with a resource
	// path from the instance metadata service
	ReadResource(path string) ([]byte, error)
	// InstanceIdentityDocument retrieves the instance identity
	// document from the instance metadata service
	InstanceIdentityDocument() (*InstanceIdentityDocument, error)
	// VPCID returns the VPC id for the network interface, given
	// its mac address
	VPCID(mac string) (string, error)
	// SubnetID returns the subnet id for the network interface,
	// given its mac address
	SubnetID(mac string) (string, error)
	// PrimaryENIMAC returns the MAC address for the primary
	// network interface of the instance
	PrimaryENIMAC() (string, error)
}

type ec2MetadataClient struct {
	httpClient http.Client
}

// NewEC2MetadataClient creates a new ec2MetadataClient object
func NewEC2MetadataClient(httpClient http.Client) EC2MetadataClient {
	if httpClient == nil {
		var lowTimeoutDial net_http.RoundTripper = &net_http.Transport{
			Dial: (&net.Dialer{
				Timeout: EC2MetadataRequestTimeout,
			}).Dial,
		}

		httpClient = &net_http.Client{Transport: lowTimeoutDial}
	}

	return &ec2MetadataClient{httpClient: httpClient}
}

func (client *ec2MetadataClient) InstanceIdentityDocument() (*InstanceIdentityDocument, error) {
	rawIIDResponse, err := client.ReadResource(InstanceIdentityDocumentResource)
	if err != nil {
		return nil, err
	}

	var iid InstanceIdentityDocument

	err = json.Unmarshal(rawIIDResponse, &iid)
	if err != nil {
		return nil, err
	}
	return &iid, nil
}

func (client *ec2MetadataClient) ReadResource(path string) ([]byte, error) {
	endpoint := client.resourceServiceURL(path)

	var err error
	var resp *net_http.Response
	utils.RetryNWithBackoff(
		utils.NewSimpleBackoff(metadataRetryStartDelay,
			metadataRetryMaxDelay, metadataRetryDelayMultiple, 0.2),
		metadataRetries,
		func() error {
			resp, err = client.httpClient.Get(endpoint)
			if err == nil && resp.StatusCode == 200 {
				return nil
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if err == nil {
				seelog.Warnf("Error accessing the EC2 Metadata Service; non-200 response: %v", resp.StatusCode)
				return errors.Errorf("ec2 metadata client: unsuccessful response from Metadata service: %v", resp.StatusCode)
			}
			seelog.Warnf("Error accessing the EC2 Metadata Service; retrying: %v", err)
			return err
		})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(resp.Body)
}

func (client *ec2MetadataClient) resourceServiceURL(path string) string {
	// TODO, override EC2MetadataServiceURL based on the environment
	return EC2MetadataServiceURL + path
}

func (client *ec2MetadataClient) PrimaryENIMAC() (string, error) {
	return client.readResourceString(macResource, "MAC address of primary network interface")
}

func (client *ec2MetadataClient) readResourceString(path string, resourceName string) (string, error) {
	response, err := client.ReadResource(path)
	if err != nil {
		return "", errors.Wrapf(err,
			"ec2 metadata client: unable to determine %s", resourceName)
	}

	return string(response), nil
}

func (client *ec2MetadataClient) VPCID(mac string) (string, error) {
	return client.readResourceString(fmt.Sprintf(vpcIDResourceFormat, mac),
		fmt.Sprintf("VPC ID for MAC address: %s", mac))
}

func (client *ec2MetadataClient) SubnetID(mac string) (string, error) {
	return client.readResourceString(fmt.Sprintf(subnetIDResourceFormat, mac),
		fmt.Sprintf("Subnet ID for MAC address: %s", mac))
}
