package nswrapper

import "github.com/containernetworking/plugins/pkg/ns"

// NS wraps methods used from the cni/pkg/ns package
type NS interface {
	GetNS(nspath string) (ns.NetNS, error)
	WithNetNSPath(nspath string, toRun func(ns.NetNS) error) error
}

type agentNS struct {
}

// NewNS creates a new NS object
func NewNS() NS {
	return &agentNS{}
}

func (*agentNS) GetNS(nspath string) (ns.NetNS, error) {
	return ns.GetNS(nspath)
}

func (*agentNS) WithNetNSPath(nspath string, toRun func(ns.NetNS) error) error {
	return ns.WithNetNSPath(nspath, toRun)
}
