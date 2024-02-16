package doctor

import (
	"encoding/binary"
	"net"

	log "github.com/cihub/seelog"
)

const (
	ecsInitSocket = "/var/run/ecs.sock"
	systemctlCmd  = "systemctl is-active "
)

// runCustomHealthCheckCmd requests ECS init to run the custom healthcheck command and returns the response
func runCustomHealthCheckCmd(name string, hcType string, timeout int) string {
	var cmd string
	// parse the health check type
	switch hcType {
	case "systemd-service":
	default:
		log.Errorf("Unknown healthcheck type, defaulting to systemd-service type: %v", hcType)

	}
	cmd = systemctlCmd + name

	conn, err := net.Dial("unix", ecsInitSocket)
	if err != nil {
		log.Errorf("Error connecting to the ECS init socket: %v, err: %v", ecsInitSocket, err)
		return ""
	}
	defer conn.Close()

	// Pack together timeout and the custom healthcheck command
	request := make([]byte, 4)
	binary.LittleEndian.PutUint32(request, uint32(timeout))
	request = append(request, []byte(cmd)...)

	// send a request to ECS init
	log.Debugf("Sending custom healthcheck command to ecs-init: %v", cmd)
	_, err = conn.Write(request)
	if err != nil {
		log.Errorf("Error sending the custom healthcheck command: %v, err: %v", cmd, err)
		return ""
	}

	// Parse the response received from ECS init
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		log.Errorf("Error receiving response from ECS init: %v", err)
		return ""
	}
	return string(buf[0:n])
}
