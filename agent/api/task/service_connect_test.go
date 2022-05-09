//go:build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package task

import (
	"fmt"
	"strings"
	"testing"

	"strconv"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

const (
	testAwsVpcPortOverride                  = "8080"
	testAwsVpcPortDefault                   = "9090"
	testBridgePortOverride                  = "8080"
	testBridgePortDefault                   = "15000"
	testBridgeHostPort                      = "8080"
	testServiceConnectContainerName         = "ecs-service-connect"
	testHostName                            = "testHostName"
	testAddress                             = "testAddress"
	testOutboundListenerName                = "testOutboundListener"
	testInboundListenerName                 = "testInboundListener"
	testIPv4Address                         = "172.31.21.40"
	testIPv6Address                         = "abcd:dcba:1234:4321::"
	testIPv4Cidr                            = "127.255.0.0/16"
	testIPv6Cidr                            = "2002::1234:abcd:ffff:c0a8:101/64"
	testEgressConfigFormat                  = `\"egressConfig\":{\"listenerName\":\"%s\",\"vip\":{\"ipv4Cidr\":\"%s\",\"ipv6Cidr\":\"%s\"}}`
	testDnsConfigFormat                     = `\"dnsConfig\":[{\"hostname\":\"%s\",\"address\":\"%s\"}]`
	testIngressConfigAwsVpcDefaultFormat    = `\"ingressConfig\":[{\"interceptPort\":%s,\"listenerName\":\"%s\"}]`
	testIngressConfigBridgeOverrideFormat   = `\"ingressConfig\":[{\"listenerPort\":%s,\"hostPort\":%s}]`
	testIngressConfigListenerPortOnlyFormat = `\"ingressConfig\":[{\"listenerPort\":%s}]`
)

var (
	testAwsVpcDefaultInterceptPort        = aws.Uint16(9090)
	testListenerPort                      = uint16(8080)
	testBridgeOverrideHostPort            = aws.Uint16(8080)
	testBridgeDefaultListenerPort         = uint16(15000)
	testAwsVpcDefaultSCConfig             = ""
	testAwsVpcDefaultIPv6EnabledSCConfig  = ""
	testAwsVpcOverrideSCConfig            = ""
	testAwsVpcOverrideIPv6EnabledSCConfig = ""
	testBridgeDefaultSCConfig             = ""
	testBridgeDefaultEmptyIngressSCConfig = ""
	testBridgeDefaultEmptyEgressSCConfig  = ""
)

func initServiceConnectConfValue() {
	testAwsVpcDefaultSCConfig = constructTestServiceConnectConfig(AWSVPCNetworkMode, false, false, false, false)
	testAwsVpcDefaultIPv6EnabledSCConfig = constructTestServiceConnectConfig(AWSVPCNetworkMode, false, false, false, true)
	testAwsVpcOverrideSCConfig = constructTestServiceConnectConfig(AWSVPCNetworkMode, true, false, false, false)
	testAwsVpcOverrideIPv6EnabledSCConfig = constructTestServiceConnectConfig(AWSVPCNetworkMode, true, false, false, true)
	testBridgeDefaultSCConfig = constructTestServiceConnectConfig(BridgeNetworkMode, false, false, false, false)
	testBridgeDefaultEmptyIngressSCConfig = constructTestServiceConnectConfig(BridgeNetworkMode, false, true, false, false)
	testBridgeDefaultEmptyEgressSCConfig = constructTestServiceConnectConfig(BridgeNetworkMode, false, false, true, false)
}

// constructTestServiceConnectConfig returns service connect config value as string based on the passed values.
func constructTestServiceConnectConfig(networkMode string, override, emptyIngress, emptyEgress, ipv6Enabled bool) string {
	ingressConfig := ""
	switch networkMode {
	case AWSVPCNetworkMode:
		if override {
			// awsvpc override case has listener port(s) in the ingress config
			ingressConfig = fmt.Sprintf(testIngressConfigListenerPortOnlyFormat, testAwsVpcPortOverride)
		} else {
			// awsvpc default case has intercept port(s) and listener name(s) in the ingress config
			ingressConfig = fmt.Sprintf(testIngressConfigAwsVpcDefaultFormat, testAwsVpcPortDefault, testInboundListenerName)
		}
	case BridgeNetworkMode:
		if override {
			// bridge override case has listener port(s) and host port(s) in the ingress config
			ingressConfig = fmt.Sprintf(testIngressConfigBridgeOverrideFormat, testBridgePortOverride, testBridgeHostPort)
		} else {
			// bridge default case has listener port(s) in the ingress config
			ingressConfig = fmt.Sprintf(testIngressConfigListenerPortOnlyFormat, testBridgePortDefault)
		}
	}

	testEgressConfig := fmt.Sprintf(testEgressConfigFormat, testOutboundListenerName, testIPv4Cidr, "")
	testDnsConfig := fmt.Sprintf(testDnsConfigFormat, testHostName, testIPv4Address)
	if ipv6Enabled {
		testEgressConfig = fmt.Sprintf(testEgressConfigFormat, testOutboundListenerName, "", testIPv6Cidr)
		testDnsConfig = fmt.Sprintf(testDnsConfigFormat, testHostName, testIPv6Address)
	}

	testServiceConnectConfig := strings.Join([]string{`"{`,
		testEgressConfig + `,`,
		testDnsConfig + `,`,
		ingressConfig,
		`}"`,
	}, "")

	if emptyIngress {
		testServiceConnectConfig = strings.Join([]string{`"{`,
			testEgressConfig + `,`,
			testDnsConfig,
			`}"`,
		}, "")
	}

	if emptyEgress {
		testServiceConnectConfig = strings.Join([]string{`"{`,
			ingressConfig,
			`}"`,
		}, "")
	}

	unquotedSCConfig, _ := strconv.Unquote(testServiceConnectConfig)
	return unquotedSCConfig
}

// getTestACSAttachmentProperty returns *ecsacs.AttachmentProperty with passed parameters.
func getTestACSAttachmentProperty(propertyName, propertyValue string) *ecsacs.AttachmentProperty {
	return &ecsacs.AttachmentProperty{
		Name:  strptr(propertyName),
		Value: strptr(propertyValue),
	}
}

// getTestACSAttachments returns *ecsacs.Task.getTestACSAttachments.
func getTestACSAttachments(attachmentProperties []*ecsacs.AttachmentProperty) *ecsacs.Attachment {
	return &ecsacs.Attachment{
		AttachmentArn:        strptr("attachmentArn"),
		AttachmentProperties: attachmentProperties,
		AttachmentType:       strptr(serviceConnectAttachmentType),
	}
}

// getExpectedTestServiceConnectConfig returns *ServiceConnectConfig based on given parameters.
func getExpectedTestServiceConnectConfig(scContainerName string,
	scIngressConfig []IngressConfigEntry,
	scEgressConfig *EgressConfig,
	scDNSConfig []DNSConfigEntry) *ServiceConnectConfig {
	return &ServiceConnectConfig{
		ContainerName: scContainerName,
		IngressConfig: scIngressConfig,
		EgressConfig:  scEgressConfig,
		DNSConfig:     scDNSConfig,
	}
}

func TestParseServiceConnectAttachment(t *testing.T) {
	initServiceConnectConfValue()
	testSCContainerNameAttachmentProperty := getTestACSAttachmentProperty(serviceConnectContainerNameKey, testServiceConnectContainerName)
	tt := []struct {
		testName                 string
		testSCAttachmentProperty *ecsacs.AttachmentProperty
		testNetworkMode          string
		ipv6Enabled              bool
		expectedIngressConfig    []IngressConfigEntry
		expectedEgressConfig     *EgressConfig
		expectedDnsConfig        []DNSConfigEntry
	}{
		{
			testName:                 "AWSVPC default case",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testAwsVpcDefaultSCConfig),
			testNetworkMode:          AWSVPCNetworkMode,
			ipv6Enabled:              false,
			expectedIngressConfig: []IngressConfigEntry{
				{
					InterceptPort: testAwsVpcDefaultInterceptPort,
					ListenerName:  testInboundListenerName,
				},
			},
			expectedEgressConfig: &EgressConfig{
				ListenerName: testOutboundListenerName,
				VIP: VIP{
					IPV4CIDR: testIPv4Cidr,
					IPV6CIDR: "",
				},
			},
			expectedDnsConfig: []DNSConfigEntry{
				{
					HostName: testHostName,
					Address:  testIPv4Address,
				},
			},
		},
		{
			testName:                 "AWSVPC default case with IPv6 enabled",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testAwsVpcDefaultIPv6EnabledSCConfig),
			testNetworkMode:          AWSVPCNetworkMode,
			ipv6Enabled:              true,
			expectedIngressConfig: []IngressConfigEntry{
				{
					InterceptPort: testAwsVpcDefaultInterceptPort,
					ListenerName:  testInboundListenerName,
				},
			},
			expectedEgressConfig: &EgressConfig{
				ListenerName: testOutboundListenerName,
				VIP: VIP{
					IPV4CIDR: "",
					IPV6CIDR: testIPv6Cidr,
				},
			},
			expectedDnsConfig: []DNSConfigEntry{
				{
					HostName: testHostName,
					Address:  testIPv6Address,
				},
			},
		},
		{
			testName:                 "AWSVPC override case",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testAwsVpcOverrideSCConfig),
			testNetworkMode:          AWSVPCNetworkMode,
			ipv6Enabled:              false,
			expectedIngressConfig: []IngressConfigEntry{
				{
					ListenerPort: testListenerPort,
				},
			},
			expectedEgressConfig: &EgressConfig{
				ListenerName: testOutboundListenerName,
				VIP: VIP{
					IPV4CIDR: testIPv4Cidr,
					IPV6CIDR: "",
				},
			},
			expectedDnsConfig: []DNSConfigEntry{
				{
					HostName: testHostName,
					Address:  testIPv4Address,
				},
			},
		},
		{
			testName:                 "AWSVPC override case with IPv6 enabled",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testAwsVpcOverrideIPv6EnabledSCConfig),
			testNetworkMode:          AWSVPCNetworkMode,
			ipv6Enabled:              true,
			expectedIngressConfig: []IngressConfigEntry{
				{
					ListenerPort: testListenerPort,
				},
			},
			expectedEgressConfig: &EgressConfig{
				ListenerName: testOutboundListenerName,
				VIP: VIP{
					IPV4CIDR: "",
					IPV6CIDR: testIPv6Cidr,
				},
			},
			expectedDnsConfig: []DNSConfigEntry{
				{
					HostName: testHostName,
					Address:  testIPv6Address,
				},
			},
		},
		{
			testName:                 "Bridge default case",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testBridgeDefaultSCConfig),
			testNetworkMode:          BridgeNetworkMode,
			ipv6Enabled:              false,
			expectedIngressConfig: []IngressConfigEntry{
				{
					ListenerPort: testBridgeDefaultListenerPort,
				},
			},
			expectedEgressConfig: &EgressConfig{
				ListenerName: testOutboundListenerName,
				VIP: VIP{
					IPV4CIDR: testIPv4Cidr,
					IPV6CIDR: "",
				},
			},
			expectedDnsConfig: []DNSConfigEntry{
				{
					HostName: testHostName,
					Address:  testIPv4Address,
				},
			},
		},
		{
			testName:                 "Bridge default case with no ingress config",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testBridgeDefaultEmptyIngressSCConfig),
			testNetworkMode:          BridgeNetworkMode,
			ipv6Enabled:              false,
			expectedEgressConfig: &EgressConfig{
				ListenerName: testOutboundListenerName,
				VIP: VIP{
					IPV4CIDR: testIPv4Cidr,
					IPV6CIDR: "",
				},
			},
			expectedDnsConfig: []DNSConfigEntry{
				{
					HostName: testHostName,
					Address:  testIPv4Address,
				},
			},
		},
		{
			testName:                 "Bridge default case with no egress config and dns config",
			testSCAttachmentProperty: getTestACSAttachmentProperty(serviceConnectConfigKey, testBridgeDefaultEmptyEgressSCConfig),
			testNetworkMode:          BridgeNetworkMode,
			ipv6Enabled:              false,
			expectedIngressConfig: []IngressConfigEntry{
				{
					ListenerPort: testBridgeDefaultListenerPort,
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			expectedTestServiceConnectConfig := getExpectedTestServiceConnectConfig(testServiceConnectContainerName,
				tc.expectedIngressConfig,
				tc.expectedEgressConfig,
				tc.expectedDnsConfig)
			testAttachmentProperties := []*ecsacs.AttachmentProperty{testSCContainerNameAttachmentProperty}
			testAttachmentProperties = append(testAttachmentProperties, tc.testSCAttachmentProperty)
			testSCAttachment := getTestACSAttachments(testAttachmentProperties)
			parsedServiceConnectConfig, err := ParseServiceConnectAttachment(testSCAttachment, tc.testNetworkMode, tc.ipv6Enabled)
			assert.NoError(t, err)
			assert.Equal(t, expectedTestServiceConnectConfig, parsedServiceConnectConfig)
		})
	}
}

func TestParseServiceConnectAttachmentWithError(t *testing.T) {
	initServiceConnectConfValue()
	testSCAttachmentProperty := getTestACSAttachmentProperty(serviceConnectConfigKey, testAwsVpcDefaultSCConfig)
	tt := []struct {
		testName                    string
		testSCContainerName         string
		testAttachmentPropertyValue string
	}{
		{
			testName:                    "AWSVPC default case with the invalid attachment property value",
			testSCContainerName:         testServiceConnectContainerName,
			testAttachmentPropertyValue: "////hellooooooo////worlddddddd",
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			testSCAttachmentProperty = getTestACSAttachmentProperty(serviceConnectConfigKey, tc.testAttachmentPropertyValue)
			testSCContainerNameAttachmentProperty := getTestACSAttachmentProperty(serviceConnectContainerNameKey, tc.testSCContainerName)
			testAttachmentProperties := []*ecsacs.AttachmentProperty{testSCAttachmentProperty}
			testAttachmentProperties = append(testAttachmentProperties, testSCContainerNameAttachmentProperty)
			testSCAttachment := getTestACSAttachments(testAttachmentProperties)
			_, err := ParseServiceConnectAttachment(testSCAttachment, AWSVPCNetworkMode, false)
			assert.Error(t, err)
		})
	}
}
