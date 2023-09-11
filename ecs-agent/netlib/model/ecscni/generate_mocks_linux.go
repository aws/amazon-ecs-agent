//go:build !windows
// +build !windows

package ecscni

//go:generate mockgen -build_flags=--mod=mod -destination=mocks/ecscni_mocks_linux.go github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni Config,NetNSUtil,CNI
