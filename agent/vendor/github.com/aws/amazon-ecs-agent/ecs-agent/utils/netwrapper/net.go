package netwrapper

import (
	n "net"
)

type Net interface {
	Interfaces() ([]n.Interface, error)
	InterfaceByName(intfName string) (*n.Interface, error)
	Addrs(intf *n.Interface) ([]n.Addr, error)
}

type net struct {
}

func NewNet() Net {
	return &net{}
}

func (*net) Interfaces() ([]n.Interface, error) {
	return n.Interfaces()
}

func (*net) InterfaceByName(intfName string) (*n.Interface, error) {
	return n.InterfaceByName(intfName)
}
func (*net) Addrs(intf *n.Interface) ([]n.Addr, error) {
	return intf.Addrs()
}
