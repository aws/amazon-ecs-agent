package main

import (
	"github.com/deniswernert/udev"
	"log"
)

func main() {
	monitor, err := udev.NewMonitor()
	if nil != err {
		log.Fatalln(err)
	}

	defer monitor.Close()
	events := make(chan *udev.UEvent)
	monitor.Monitor(events)
	for {
		event := <-events
		log.Println(event.String())
	}
}
