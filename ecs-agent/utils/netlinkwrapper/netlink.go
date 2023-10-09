package netlinkwrapper

import (
	"github.com/vishvananda/netlink"
)

type NetLink interface {
	LinkByName(name string) (netlink.Link, error)
	LinkSetUp(link netlink.Link) error
}

type netLink struct{}

func New() NetLink {
	return &netLink{}
}

func (nl *netLink) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (nl *netLink) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}
