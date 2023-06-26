package testconst

// This file contains constants that are commonly used when testing ACS session and responders. These constants
// should only be called in test files.
const (
	ClusterName          = "default"
	ContainerInstanceARN = "instance"
	TaskARN              = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
	MessageID            = "123"
	RandomMAC            = "00:0a:95:9d:68:16"
	WaitTimeoutMillis    = 1000
	InterfaceProtocol    = "default"
	GatewayIPv4          = "192.168.1.1/24"
	IPv4Address          = "ipv4"
)
