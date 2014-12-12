package dialer

import (
	"fmt"
	"io"
	"net"

	"crypto/tls"
)

type Dialer interface {
	//Dial creates a new connection to the service
	Dial() (io.ReadWriter, error)
}

type DialerFunc func() (io.ReadWriter, error)

func (f DialerFunc) Dial() (io.ReadWriter, error) {
	return f()
}

//TCP creates a Dialer to the address and port using a TCP connection
func TCP(addr string, port uint16) (Dialer, error) {
	address := fmt.Sprintf("%s:%d", addr, port)
	resolved, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}
	return DialerFunc(func() (io.ReadWriter, error) {
		return net.DialTCP("tcp", nil, resolved)
	}), nil
}

//TLS creates a Dialer to the address and port using a TLS (SSL) connection
func TLS(addr string, port uint16, config *tls.Config) (Dialer, error) {
	address := fmt.Sprintf("%s:%d", addr, port)
	return DialerFunc(func() (io.ReadWriter, error) {
		return tls.Dial("tcp", address, config)
	}), nil
}
