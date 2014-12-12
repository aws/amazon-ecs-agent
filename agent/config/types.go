package config

type Config struct {
	ClusterArn     string
	APIEndpoint    string
	APIPort        uint16
	DockerEndpoint string
	AWSRegion      string `missing:"warn"`
	ReservedPorts  []uint16
}
