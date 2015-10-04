package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	lxc "gopkg.in/lxc/go-lxc.v2"
)

func main() {
	fname := flag.String("from", "", "name of container to clone from or create before cloning")
	bname := flag.String("to", "", "name to use for clone")

	if *fname == "" || *bname == "" {
		log.Println("Unacceptable for names to be empty!")
		return
	}

	var err error

	cl := &lxc.CloneOptions{
		Backend:  lxc.Aufs,
		KeepName: true,
		Snapshot: true,
	}

	tl := &lxc.TemplateOptions{
		Template: "download",
		Distro:   "ubuntu",
		Release:  "trusty",
		Arch:     "amd64",
		Backend:  lxc.Directory,
	}

	c, err := lxc.NewContainer(*fname, lxc.DefaultConfigPath())

	if err != nil {
		log.Printf("Allocating Container %s error %s", *fname, err)
		panic("Unable to get container")
	}

	lxc.Release(c)

	if !c.Defined() {

		err := c.Create(*tl)

		if err != nil {
			log.Printf("Allocating Container %s error %s", *fname, err)
			panic("Unable to create container")
		}
	}

	err = c.Clone(*bname, *cl)

	if err != nil {
		log.Println("Unable to clone", *bname)
		return
	}

	cloned, err := lxc.NewContainer(*bname, lxc.DefaultConfigPath())

	if err != nil {
		log.Println("Unable to locate clone", *bname)
		return
	}

	switch cloned.State() {
	case lxc.FROZEN:
		err = cloned.Unfreeze()
		log.Printf("Container in current State: %+s", cloned.State())
	case lxc.STOPPED:
		err = cloned.Start()
		log.Printf("Container in current State: %+s", cloned.State())
	case lxc.RUNNING:
		log.Printf("Container in current State: %+s", cloned.State())
		return
	default:
		log.Printf("Container in current State: %+s", cloned.State())
		err = fmt.Errorf("Bad controller")
	}

	if err != nil {
		return
	}

	if !cloned.Wait(lxc.RUNNING, 60) {
		log.Printf("Unable to get it running (%+v)", cloned.State())
		return
	}

	// var dest string
	//
	// log.Printf("Begin IP Acquisition process for container")
	//
	// for {
	// 	ip, err := con.IPAddress("eth0")
	//
	// 	if err != nil {
	// 		log.Printf("Waiting for Container (%s) IP to settle", err.Error())
	// 		time.Sleep(time.Millisecond * time.Duration(1000))
	// 		continue
	// 	}
	//
	// 	break
	// }
	//
	cloned.Freeze()

	if !cloned.Wait(lxc.FROZEN, 60) {
		log.Printf("Unable to freeze it (%+s)", *bname)
		return
	}

	var durs int

	fmt.Scan(&durs)

	time.Sleep(time.Duration(durs) * time.Second)

	lxc.Release(cloned)

	time.Sleep(time.Duration(durs) * time.Second)

}
