package agent_tls

import (
	"crypto/tls"
)
// Only support a subset of ciphers, corresponding cipher suite names can be found here: https://golang.org/pkg/crypto/tls/#Config
var SupportedCipherSuites = []uint16{0xc02f, 0xc027, 0xc013, 0xc030, 0xc014, 0x009c, 0x003c, 0x002f, 0x009d, 0x0035}

func SetCipherCuites(config *tls.Config) {
	config.CipherSuites = SupportedCipherSuites
}