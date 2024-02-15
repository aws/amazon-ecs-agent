package doctor

import (
	"encoding/binary"
	"fmt"
	log "github.com/cihub/seelog"
	"net"
	"os"
)

const (
	ecsInitSocket = "/var/run/ecs.sock"
)

type CustomHealthCheckClient interface {
	runCustomHealthCheckCmd(string) string
}

type CustomHealthCheckGoClient struct {
}

func runCustomHealthCheckCmd(commandMsg string, timeout int) string {
	conn, err := net.Dial("unix", ecsInitSocket)
	if err != nil {
		fmt.Println("Error connecting:", err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	// Pack together timeout and the command message to send it
	byteMessage := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteMessage, uint32(timeout))
	byteMessage = append(byteMessage, []byte(commandMsg)...)

	log.Infof("Sending custom healthcheck command to ecs-init: %v", commandMsg)
	_, err = conn.Write(byteMessage)
	if err != nil {
		log.Errorf("Error while sending the custom healthcheck command: %v, err: %v", commandMsg, err)
		return ""
	}

	// Wait for response
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error receiving response:", err.Error())
		return ""
	}

	response := string(buf[0:n])
	return response
}
