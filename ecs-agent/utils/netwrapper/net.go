package netwrapper

import (
	n "net"
)

type Net interface {
	Interfaces() ([]n.Interface, error)
}

type net struct {
}

func NewNet() Net {
	return &net{}
}

func (*net) Interfaces() ([]n.Interface, error) {
	return n.Interfaces()
}
