package serviceconnect

import (
	"fmt"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	dockercontainer "github.com/docker/docker/api/types/container"
)

func AppendEgressVIPHosts(dnsConf []apitask.DNSConfig, dockerHostConf *dockercontainer.HostConfig) {
	for _, dnsCfg := range dnsConf {
		if len(dnsCfg.IPV4Address) > 0 {
			dockerHostConf.ExtraHosts = append(dockerHostConf.ExtraHosts,
				fmt.Sprintf("%s:%s", dnsCfg.HostName, dnsCfg.IPV4Address))
		}
		if len(dnsCfg.IPV6Address) > 0 {
			dockerHostConf.ExtraHosts = append(dockerHostConf.ExtraHosts,
				fmt.Sprintf("%s:%s", dnsCfg.HostName, dnsCfg.IPV6Address))
		}
	}
}
