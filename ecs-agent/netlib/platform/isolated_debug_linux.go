package platform

import (
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/pkg/errors"
)

type isolatedDebug struct {
	isolatedLinux
}

func (d *isolatedDebug) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	if err := d.createDNSConfig(taskID, true, netNS); err != nil {
		return err
	}

	// Backfill interface DNS fields from the copied resolv.conf so downstream
	// consumers can read DNS config from the model.
	primaryIF := netNS.GetPrimaryInterface()
	if primaryIF == nil || len(primaryIF.DomainNameServers) > 0 {
		return nil
	}

	src := filepath.Join(d.resolvConfPath, ResolveConfFileName)
	contents, err := d.ioutil.ReadFile(src)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", src)
	}
	servers, searches := parseResolvConf(contents)
	primaryIF.DomainNameServers = servers
	primaryIF.DomainNameSearchList = searches
	return nil
}
