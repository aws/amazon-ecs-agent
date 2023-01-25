// +build linux

package udev

import (
	"os"
	"syscall"
)

// UDevMonitor is a UDev netlink socket
type UDevMonitor struct {
	Fd      int
	Address syscall.SockaddrNetlink
}

// NewMonitor creates and connects a new monitor
func NewMonitor() (mon *UDevMonitor, err error) {
	mon = new(UDevMonitor)
	if err = mon.Connect(); err != nil {
		mon = nil
	}
	return
}

// Connect connects the monitoring socket
func (mon *UDevMonitor) Connect() (err error) {
	mon.Fd, err = syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM, syscall.NETLINK_KOBJECT_UEVENT)
	if nil != err {
		return
	}

	mon.Address = syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: 4294967295,
		Pid:    uint32(os.Getpid()),
	}

	err = syscall.Bind(mon.Fd, &mon.Address)
	if nil != err {
		syscall.Close(mon.Fd)
	}

	return
}

// Close closes the monitor socket
func (mon *UDevMonitor) Close() error {
	return syscall.Close(mon.Fd)
}

func (mon *UDevMonitor) Read(b []byte) (n int, err error) {
	return syscall.Read(mon.Fd, b)
}

// Process processes one packet from the socket, and sends the event on the notify channel
func (mon *UDevMonitor) Process(notify chan *UEvent) {
	buffer := make([]byte, syscall.Getpagesize())
	bytesRead, from, err := syscall.Recvfrom(mon.Fd, buffer, 0)
	switch v := from.(type) {
	case *syscall.SockaddrNetlink:
		if v.Pid != 0 {
			// Filter out non-kernel messages.
			// TODO: this should probably be configurable
			return
		}

	default:
		// Shouldn't happen, we might want to panic here actually
		return
	}

	if bytesRead <= 0 {
		return
	}

	if nil == err {
		buffer = buffer[:bytesRead]
		if parsed, err := ParseUEvent(buffer); err == nil {
			notify <- parsed
		}
	}
}

// Monitor starts udev event monitoring. Events are sent on the notify channel, and the watch can be
// terminated by sending true on the returned shutdown channel
func (mon *UDevMonitor) Monitor(notify chan *UEvent) (shutdown chan bool) {
	shutdown = make(chan bool)
	go func() {
	done:
		for {
			select {
			case <-shutdown:
				break done
			default:
				mon.Process(notify)
			}
		}
	}()
	return shutdown
}
