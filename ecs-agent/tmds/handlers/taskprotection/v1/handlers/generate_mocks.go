package handlers

//go:generate mockgen -destination=handlers_mocks.go -package=handlers -copyright_file=../../../../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/taskprotection/v1/handlers TaskProtectionClientFactoryInterface
