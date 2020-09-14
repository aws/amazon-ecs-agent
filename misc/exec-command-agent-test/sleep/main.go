package main

import (
	"flag"
	"time"
)

func main() {
	var timeFlag = flag.Duration("time", time.Second*5, "sleep duration")
	flag.Parse()
	time.Sleep(*timeFlag)
}
