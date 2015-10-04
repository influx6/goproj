//checker provides a means to check a controller state
package main

import (
	"flag"
	"log"

	"gopkg.in/lxc/go-lxc.v2"
)

func main() {
	tag := flag.String("id", "", "the name of the container to check")

	flag.Parse()

	if *tag == "" {
		log.Println("No tag Supplied")
		return
	}

	c, err := lxc.NewContainer(*tag, lxc.DefaultConfigPath())

	if err != nil {
		log.Fatalf("Unable to gained controller: %+s", err)
	}

	ack := lxc.Acquire(c)

	log.Printf("Printing Out Container %s Info........", *tag)
	log.Printf("Is Acquired: %t", ack)

	switch c.State() {
	case lxc.FROZEN:
		log.Printf("State: FROZEN")
	case lxc.RUNNING:
		log.Printf("State: RUNNING")
	case lxc.STARTING:
		log.Printf("State: STARTING")
	case lxc.ABORTING:
		log.Printf("State: ABORTING")
	case lxc.FREEZING:
		log.Printf("State: FREEZING")
	case lxc.THAWED:
		log.Printf("State: THAWED")
	case lxc.STOPPED:
		log.Printf("State: STOPPED")
	case lxc.STOPPING:
		log.Printf("State: STOPPING")
	}
}
