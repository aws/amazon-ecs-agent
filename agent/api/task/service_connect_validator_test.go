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
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

var (
	testTaskContainers = []*ecsacs.Container{
		{
			Name: aws.String(testServiceConnectContainerName),
		},
		{
			Name: aws.String("testTaskFirstContainerName"),
		},
		{
			Name: aws.String("testTaskSecondContainerName"),
		},
	}
)

// getTestServiceConnectConfig returns *ServiceConnectConfig based on given parameters.
func getTestServiceConnectConfig(
	scContainerName string,
	scEgressConfig *EgressConfig,
	scDnsConfig []DNSConfigEntry,
	scIngressConfig []IngressConfigEntry) *ServiceConnectConfig {
	return &ServiceConnectConfig{
		ContainerName: scContainerName,
		IngressConfig: scIngressConfig,
		EgressConfig:  scEgressConfig,
		DNSConfig:     scDnsConfig,
	}
}

// getTestEgressConfig returns *EgressConfig based on given parameters.
func getTestEgressConfig(scListenerName, scIPv4Cidr, scIPv6Cidr string) *EgressConfig {
	return &EgressConfig{
		ListenerName: scListenerName,
		VIP: VIP{
			IPV4CIDR: scIPv4Cidr,
			IPV6CIDR: scIPv6Cidr,
		},
	}
}

// getTestDnsConfig returns DNSConfigEntry based on given parameters.
func getTestDnsConfigEntry(scHostname, scAddress string) DNSConfigEntry {
	return DNSConfigEntry{
		HostName: testHostName,
		Address:  scAddress,
	}
}

// getTestIngressConfigEntry returns IngressConfigEntry based on given parameters.
func getTestIngressConfigEntry(networkMode, scListenerName string,
	override bool,
	scListenerPort uint16,
	scInterceptPort, scHostPort *uint16) IngressConfigEntry {
	var entry IngressConfigEntry
	switch networkMode {
	case AWSVPCNetworkMode:
		if override {
			// awsvpc override case has listener port(s) in the ingress config
			entry = IngressConfigEntry{
				ListenerPort: scListenerPort,
			}
		} else {
			// awsvpc default case has intercept port(s) and listener name(s) in the ingress config
			entry = IngressConfigEntry{
				ListenerName:  scListenerName,
				InterceptPort: scInterceptPort,
			}
		}
	case BridgeNetworkMode:
		if override {
			// bridge override case has listener port(s) and host port(s) in the ingress config
			entry = IngressConfigEntry{
				ListenerPort: scListenerPort,
				HostPort:     scHostPort,
			}
		} else {
			// bridge default case has listener port(s) in the ingress config
			entry = IngressConfigEntry{
				ListenerPort: scListenerPort,
			}
		}
	}
	return entry
}
func TestValidateServiceConnectConfig(t *testing.T) {
	tt := []struct {
		testName               string
		testNetworkMode        string
		testIsIPv6Enabled      bool
		testEgressConfig       *EgressConfig
		testDnsConfigEntry     DNSConfigEntry
		testIngressConfigEntry IngressConfigEntry
	}{
		{
			testName:               "AWSVPC default case",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC default case with IPv6 enabled",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      true,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, testIPv6Cidr),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv6Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC override case",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, "", true, testListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC override case with IPv6 enabled",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      true,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, testIPv6Cidr),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv6Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, "", true, testListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "Bridge default case",
			testNetworkMode:        BridgeNetworkMode,
			testIsIPv6Enabled:      false,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", false, testBridgeDefaultListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "Bridge default case with IPv6 enabled",
			testNetworkMode:        BridgeNetworkMode,
			testIsIPv6Enabled:      true,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, testIPv6Cidr),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv6Address),
			testIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", false, testBridgeDefaultListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "Bridge override case",
			testNetworkMode:        BridgeNetworkMode,
			testIsIPv6Enabled:      false,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
		},
		{
			testName:               "Bridge override case with IPv6 enabled",
			testNetworkMode:        BridgeNetworkMode,
			testIsIPv6Enabled:      true,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, testIPv6Cidr),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv6Address),
			testIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			dnsConfig := []DNSConfigEntry{}
			dnsConfig = append(dnsConfig, tc.testDnsConfigEntry)
			ingressConfig := []IngressConfigEntry{}
			ingressConfig = append(ingressConfig, tc.testIngressConfigEntry)
			testServiceConnectConfig := getTestServiceConnectConfig(
				testServiceConnectContainerName,
				tc.testEgressConfig,
				dnsConfig,
				ingressConfig,
			)
			err := ValidateServiceConnectConfig(testServiceConnectConfig, testTaskContainers, tc.testNetworkMode, tc.testIsIPv6Enabled)
			assert.NoError(t, err)
		})
	}
}

func TestValidateServiceConnectConfigWithWarning(t *testing.T) {
	tt := []struct {
		testName                     string
		testNetworkMode              string
		testEgressConfig             *EgressConfig
		testDnsConfigEntry           DNSConfigEntry
		testFirstIngressConfigEntry  IngressConfigEntry
		testSecondIngressConfigEntry IngressConfigEntry
	}{
		{
			testName:                     "AWSVPC default case with both intercept port and host port in the ingress config",
			testNetworkMode:              AWSVPCNetworkMode,
			testEgressConfig:             getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:           getTestDnsConfigEntry(testHostName, testIPv4Address),
			testFirstIngressConfigEntry:  getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
			testSecondIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
		},
		{
			testName:                     "Bridge override case with both host port and intercept port in the ingress config",
			testNetworkMode:              BridgeNetworkMode,
			testEgressConfig:             getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:           getTestDnsConfigEntry(testHostName, testIPv4Address),
			testFirstIngressConfigEntry:  getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
			testSecondIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			dnsConfig := []DNSConfigEntry{}
			dnsConfig = append(dnsConfig, tc.testDnsConfigEntry)
			ingressConfig := []IngressConfigEntry{}
			ingressConfig = append(ingressConfig, tc.testFirstIngressConfigEntry)
			ingressConfig = append(ingressConfig, tc.testSecondIngressConfigEntry)
			testServiceConnectConfig := getTestServiceConnectConfig(
				testServiceConnectContainerName,
				tc.testEgressConfig,
				dnsConfig,
				ingressConfig,
			)
			err := ValidateServiceConnectConfig(testServiceConnectConfig, testTaskContainers, tc.testNetworkMode, false)
			assert.NoError(t, err)
		})
	}
}

func TestValidateServiceConnectConfigWithEmptyConfig(t *testing.T) {
	var testEgressConfig *EgressConfig
	var testDnsConfig []DNSConfigEntry
	var testIngressConfig []IngressConfigEntry
	tt := []struct {
		testName                 string
		testEgressConfigIsEmpty  bool
		testIngressConfigIsEmpty bool
		shouldError              bool
	}{
		{
			testName:                 "AWSVPC default case with the empty egress config and dns config",
			testEgressConfigIsEmpty:  true,
			testIngressConfigIsEmpty: false,
			shouldError:              false,
		},
		{
			testName:                 "AWSVPC default case with the empty ingress config",
			testEgressConfigIsEmpty:  false,
			testIngressConfigIsEmpty: true,
			shouldError:              false,
		},
		{
			testName:                 "AWSVPC default case with the empty ingress config and engress config",
			testEgressConfigIsEmpty:  true,
			testIngressConfigIsEmpty: true,
			shouldError:              true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			if tc.testEgressConfigIsEmpty {
				testEgressConfig = nil
				testDnsConfig = []DNSConfigEntry{}
			} else {
				testEgressConfig = getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, "")
				testDnsConfig = append(testDnsConfig, getTestDnsConfigEntry(testHostName, testIPv4Address))
			}

			if tc.testIngressConfigIsEmpty {
				testIngressConfig = []IngressConfigEntry{}
			} else {
				testIngressConfig = append(testIngressConfig, getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)))
			}

			testServiceConnectConfig := getTestServiceConnectConfig(
				testServiceConnectContainerName,
				testEgressConfig,
				testDnsConfig,
				testIngressConfig,
			)
			err := ValidateServiceConnectConfig(testServiceConnectConfig, testTaskContainers, AWSVPCNetworkMode, false)
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateServiceConnectConfigWithError(t *testing.T) {
	tt := []struct {
		testName               string
		testNetworkMode        string
		testIsIPv6Enabled      bool
		testContainerName      string
		testEgressConfig       *EgressConfig
		testDnsConfigEntry     DNSConfigEntry
		testIngressConfigEntry IngressConfigEntry
	}{
		{
			testName:               "AWSVPC default case with no service connect container name",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testContainerName:      "",
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC default case with the service connect container name not exists in the task",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testContainerName:      "helloworld",
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC default case with no listener name in the egress config",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig("", testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC default case with no IPv4 CIDR in the egress config when Ipv6 is not enabled",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, "", ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC default case with no IPv6 CIDR in the egress config when IPv6 is enabled",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      true,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv6Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC override case with the invalid IPv4 CIDR in the egress config",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      false,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, "999999999", ""),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv4Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, "", true, testListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "AWSVPC override case with the invalid IPv6 CIDR in the egress config when IPv6 is enabled",
			testNetworkMode:        AWSVPCNetworkMode,
			testIsIPv6Enabled:      true,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, "999999999"),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, testIPv6Address),
			testIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, "", true, testListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "Bridge default case with the egress config but no dns config",
			testNetworkMode:        BridgeNetworkMode,
			testIsIPv6Enabled:      false,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, "", testIPv6Cidr),
			testDnsConfigEntry:     DNSConfigEntry{},
			testIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", false, testBridgeDefaultListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:               "Bridge override case with no the invalid address in dns config when IPv6 is enabled",
			testNetworkMode:        BridgeNetworkMode,
			testIsIPv6Enabled:      true,
			testContainerName:      testServiceConnectContainerName,
			testEgressConfig:       getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, testIPv6Cidr),
			testDnsConfigEntry:     getTestDnsConfigEntry(testHostName, "999999999"),
			testIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			dnsConfig := []DNSConfigEntry{}
			dnsConfig = append(dnsConfig, tc.testDnsConfigEntry)
			ingressConfig := []IngressConfigEntry{}
			ingressConfig = append(ingressConfig, tc.testIngressConfigEntry)

			testServiceConnectConfig := getTestServiceConnectConfig(
				tc.testContainerName,
				tc.testEgressConfig,
				dnsConfig,
				ingressConfig,
			)
			err := ValidateServiceConnectConfig(testServiceConnectConfig, testTaskContainers, tc.testNetworkMode, tc.testIsIPv6Enabled)
			assert.Error(t, err)
		})
	}
}

func TestValidateServiceConnectConfigWithPortCollision(t *testing.T) {
	testEgressConfig := getTestEgressConfig(testOutboundListenerName, testIPv4Cidr, "")
	testDnsConfigEntry := getTestDnsConfigEntry(testHostName, testIPv4Address)
	tt := []struct {
		testName                     string
		testNetworkMode              string
		testFirstIngressConfigEntry  IngressConfigEntry
		testSecondIngressConfigEntry IngressConfigEntry
	}{
		{
			testName:                     "AWSVPC default case with the intercept port collision in the ingress config",
			testNetworkMode:              AWSVPCNetworkMode,
			testFirstIngressConfigEntry:  getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
			testSecondIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, testInboundListenerName, false, uint16(0), testAwsVpcDefaultInterceptPort, aws.Uint16(0)),
		},
		{
			testName:                     "AWSVPC override case with the listener port collision in the ingress config",
			testNetworkMode:              AWSVPCNetworkMode,
			testFirstIngressConfigEntry:  getTestIngressConfigEntry(AWSVPCNetworkMode, "", true, testListenerPort, aws.Uint16(0), aws.Uint16(0)),
			testSecondIngressConfigEntry: getTestIngressConfigEntry(AWSVPCNetworkMode, "", true, testListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:                     "Bridge default case with the listener port collision in the ingress config",
			testNetworkMode:              BridgeNetworkMode,
			testFirstIngressConfigEntry:  getTestIngressConfigEntry(BridgeNetworkMode, "", false, testBridgeDefaultListenerPort, aws.Uint16(0), aws.Uint16(0)),
			testSecondIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", false, testBridgeDefaultListenerPort, aws.Uint16(0), aws.Uint16(0)),
		},
		{
			testName:                     "Bridge override case with the host port collision in the ingress config",
			testNetworkMode:              BridgeNetworkMode,
			testFirstIngressConfigEntry:  getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
			testSecondIngressConfigEntry: getTestIngressConfigEntry(BridgeNetworkMode, "", true, testListenerPort, aws.Uint16(0), testBridgeOverrideHostPort),
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			dnsConfig := []DNSConfigEntry{testDnsConfigEntry}
			ingressConfig := []IngressConfigEntry{}
			ingressConfig = append(ingressConfig, tc.testFirstIngressConfigEntry)
			ingressConfig = append(ingressConfig, tc.testSecondIngressConfigEntry)
			testServiceConnectConfig := getTestServiceConnectConfig(
				testServiceConnectContainerName,
				testEgressConfig,
				dnsConfig,
				ingressConfig,
			)
			err := ValidateServiceConnectConfig(testServiceConnectConfig, testTaskContainers, tc.testNetworkMode, false)
			assert.Error(t, err)
		})
	}
}
