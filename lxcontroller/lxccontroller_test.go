package lxcontroller

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"gopkg.in/lxc/go-lxc.v2"
)

func checkTestError(t *testing.T, e error, msg string) {
	if e != nil {
		t.Fatalf("Error Occcured: (%+v) For: (%s)", e, msg)
	}
}

func TestExistingController(t *testing.T) {
	name := "verhoef"
	ok := HasController(name, lxc.DefaultConfigPath())

	if !ok {
		checkTestError(t, errors.New("Container Not found!"), fmt.Sprintf("Checking if %s container exists!", name))
	}

	t.Log("OK:", ok, name)
}

func TestNewController(t *testing.T) {
	name := "verhoef"
	wait := new(sync.WaitGroup)
	ms := time.Duration(1) * time.Minute
	prof := TemplateProfile(lxc.Directory, "download", "ubuntu", "trusty", "amd64", false, ms)
	killer := make(chan struct{})
	con, err := NewController(name, prof, DefaultConfig())

	checkTestError(t, err, fmt.Sprintf("Creating Controller %s", name))

	// t.Log("Succcess creating container for: ", name, con)

	go func() {
		<-killer
		err := con.Drop()
		log.Printf("Dropping")
		checkTestError(t, err, fmt.Sprintf("Droping Controller %s", name))
		wait.Done()
	}()

	wait.Add(1)
	err = con.Dial()
	checkTestError(t, err, fmt.Sprintf("Dialing Controller %s", name))
	log.Printf("Success Dialing! Done!")
	go func() {
		log.Println("Network:", con.NetworkInterface())
		<-time.After(time.Duration(4) * time.Second)
		close(killer)
	}()
	wait.Wait()
}

func TestControllerCloning(t *testing.T) {
	name := "verhoef"
	xname := "vandroff"
	killer := make(chan struct{})
	wait := new(sync.WaitGroup)
	ms := time.Duration(1) * time.Minute
	prof := BaseAufsProfile(nil, ms, false)
	con, err := CloneController(name, xname, prof, DefaultConfig())

	checkTestError(t, err, fmt.Sprintf("Creating Cloned Controller %s", xname))

	go func() {
		<-killer
		err := con.Drop()
		log.Printf("Dropping")
		checkTestError(t, err, fmt.Sprintf("Droping Cloned Controller %s", xname))
		wait.Done()
	}()

	wait.Add(1)
	err = con.Dial()
	checkTestError(t, err, fmt.Sprintf("Dialing Cloned Controller %s", xname))
	log.Printf("Success Dialing! Done!")
	close(killer)
	wait.Wait()
}

func TestCmdExecWithController(t *testing.T) {
	name := "verhoef"
	ms := time.Duration(7) * time.Minute
	prof := BaseAufsProfile(nil, ms, false)
	prof.Folder = "./tools"
	prof.StartupFile = "./tools/run.sh"

	con, err := NewController(name, prof, DefaultConfig())

	checkTestError(t, err, fmt.Sprintf("Creating Controller %s", name))

	err = con.Dial()

	checkTestError(t, err, fmt.Sprintf("Dialing Controller %s", name))
	// con.Drop()
}
