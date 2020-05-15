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

package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/sparrc/go-ping"
)

const (
	imdsEndpoint            = "http://169.254.169.254/latest/meta-data/"
	v2MetadataEndpoint      = "http://169.254.170.2/v2/metadata"
	v2StatsEndpoint         = "http://169.254.170.2/v2/stats"
	containerMetadataEnvVar = "ECS_CONTAINER_METADATA_URI"
	maxRetries              = 4
	durationBetweenRetries  = time.Second
)

func verifyEndPoint(client *http.Client, endPoint string) error {
	_, err := metadataResponse(client, endPoint)
	return err
}

func metadataResponse(client *http.Client, endpoint string) ([]byte, error) {
	var resp []byte
	var err error
	for i := 0; i < maxRetries; i++ {
		resp, err = metadataResponseOnce(client, endpoint)
		if err == nil {
			return resp, nil
		}
		fmt.Fprintf(os.Stderr, "attempt [%d/%d]: unable to get metadata response from '%s': %v \n",
			i, maxRetries, endpoint, err)
		time.Sleep(durationBetweenRetries)
	}

	return nil, err
}

func metadataResponseOnce(client *http.Client, endpoint string) ([]byte, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to get response: %v \n", err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("incorrect status code  %d \n", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("task metadata: unable to read response body: %v \n", err)
	}

	return body, nil
}

func verifyV2Endpoints(client *http.Client) {
	err := verifyEndPoint(client, v2MetadataEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get task metadata: %v", err)
		os.Exit(1)
	}

	err = verifyEndPoint(client, v2StatsEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get task stats: %v", err)
		os.Exit(1)
	}
}

func verifyV3Endpoints(client *http.Client) {
	v3BaseEndpoint := os.Getenv(containerMetadataEnvVar)

	taskMetadataPath := v3BaseEndpoint + "/task"

	containerStatsPath := v3BaseEndpoint + "/stats"
	taskStatsPath := v3BaseEndpoint + "/task/stats"

	if err := verifyEndPoint(client, v3BaseEndpoint); err != nil {
		fmt.Fprintf(os.Stderr, "unable to get container metadata: %v\n", err)
		os.Exit(1)
	}

	if err := verifyEndPoint(client, taskMetadataPath); err != nil {
		fmt.Fprintf(os.Stderr, "unable to get task metadata: %v\n", err)
		os.Exit(1)
	}

	if err := verifyEndPoint(client, containerStatsPath); err != nil {
		fmt.Fprintf(os.Stderr, "unable to get container stats: %v\n", err)
		os.Exit(1)
	}

	if err := verifyEndPoint(client, taskStatsPath); err != nil {
		fmt.Fprintf(os.Stderr, "unable to get task stats: %v\n", err)
		os.Exit(1)
	}
}

func verifyIAMRoleEndpoint(client *http.Client) {
	credentialsRelativeUri := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
	if len(credentialsRelativeUri) == 0 {
		fmt.Fprintf(os.Stderr, "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI environment variable missing")
		os.Exit(1)
	}
	fmt.Printf("IAM role found: %s\n", credentialsRelativeUri)

	IAMRoleEndPoint := "http://169.254.170.2" + credentialsRelativeUri
	_, err := metadataResponse(client, IAMRoleEndPoint)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error pinging IAM role endpoint:%s\n", credentialsRelativeUri)
		os.Exit(1)
	}

}

func verifyHostName() {
	cmd := exec.Command("hostname")
	hostname, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error executing `hostname` command: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Hostname Found: %s\n", string(hostname))

	regexExpPattern := "ip-[0-9]*-[0-9]*-[0-9]*-[0-9]*.*.internal"
	_, errMatch := regexp.MatchString(regexExpPattern, string(hostname))
	if errMatch != nil {
		fmt.Fprintf(os.Stderr, "incorrect format hostname: %s\n", hostname)
		os.Exit(1)
	}
}

func verifyIMDSConnectivity(client *http.Client) {
	_, err := metadataResponse(client, imdsEndpoint)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error pinging imds endpoint:%s\n", imdsEndpoint)
		os.Exit(1)
	}
}

func verifySubnetGatewayEndpoint(client *http.Client) {
	baseURL := imdsEndpoint + "network/interfaces/macs/"
	body, err := metadataResponse(client, baseURL)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error pinging endpoint: %s\n", baseURL)
		os.Exit(1)
	}
	macAddress := strings.Split(string(body), "\n")[0]
	fmt.Printf("Received mac address: %s \n", macAddress)
	pingUrl := strings.Join([]string{baseURL, macAddress, "subnet-ipv4-cidr-block"}, "")
	subnetCIDR, errSubnet := metadataResponse(client, pingUrl)
	fmt.Printf("Received CIDR: %s\n", string(subnetCIDR))

	if errSubnet != nil {
		fmt.Fprintf(os.Stderr, "error pinging endpoint: %s\n", pingUrl)
		os.Exit(1)
	}

	defaultSubnet, err := computeIPV4GatewayNetmask(string(subnetCIDR))
	if err != nil {
		fmt.Fprint(os.Stderr, "Error computing default subnet gateway IPV4: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Subnet Found: %s\n", defaultSubnet)
	pinger, errSubnetPing := ping.NewPinger(defaultSubnet)

	if errSubnetPing != nil {
		fmt.Fprintf(os.Stderr, "error pinging subnet: %s\n", errSubnetPing.Error())
		os.Exit(1)
	}

	pingerCount := 10
	packetLossAllowed := 20.0 // percentage of packet loss allowed
	pingerTimeout := 5 * time.Second

	pinger.SetPrivileged(true) // ping will run with super-user privileges
	pinger.Count = pingerCount
	pinger.Timeout = pingerTimeout

	pinger.Run()                 // blocks until finished
	stats := pinger.Statistics() // get send/receive/rtt stats
	fmt.Printf("\n--- %s ping statistics ---\n", stats.Addr)
	fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n",
		stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
	fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
		stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt)

	if stats.PacketLoss > packetLossAllowed {
		fmt.Fprint(os.Stderr, "packets were lost when pinging subnet gateway")
		os.Exit(1)
	}
}

// computeIPV4GatewayNetmask computes the subnet gateway for
// the gateway given the ipv4 cidr block for the ENI
// Gateways are provided in VPC subnets at base +1
func computeIPV4GatewayNetmask(cidrBlock string) (string, error) {
	// The IPV4 CIDR block is of the format ip-addr/netmask
	ip, _, err := net.ParseCIDR(cidrBlock)
	if err != nil {
		return "", fmt.Errorf("Failed to parse cidrBlock with error: %q", err)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return "", fmt.Errorf("unable to parse ipv4 gateway from cidr block '%s'", cidrBlock)
	}

	// ipv4 gateway is the first available IP address in the subnet
	ip4[3] = ip4[3] + 1
	return ip4.String(), nil
}

func main() {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	verifyV3Endpoints(client)
	verifyV2Endpoints(client)
	verifyIAMRoleEndpoint(client)
	verifyHostName()
	verifyIMDSConnectivity(client)
	verifySubnetGatewayEndpoint(client)

	os.Exit(42)
}
