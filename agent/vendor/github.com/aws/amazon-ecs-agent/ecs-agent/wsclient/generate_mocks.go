package wsclient

//go:generate mockgen -destination=mock/client.go -copyright_file=../../scripts/copyright_file github.com/aws/amazon-ecs-agent/ecs-agent/wsclient ClientServer,RequestResponder,ClientFactory
