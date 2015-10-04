package lxcontroller

import (
	"log"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/influx6/flux"
)

func TestSnifer(t *testing.T) {
	wait := new(sync.WaitGroup)
	snif := NewSnifer("lo", &SniferConfig{
		Filter:    "",
		Timeout:   time.Duration(3) * time.Second,
		MaxPacket: 1600,
		Promod:    true,
		Offline:   false,
	})

	defer snif.Kill()
	wait.Add(1)

	snif.Listen(func(packet gopacket.Packet, sb *flux.Sub) {
		t.Log("Received Packet:", packet)
		defer sb.Close()
		defer wait.Done()
	})

	err := snif.Live()
	checkTestError(t, err, "Unable to create snifer")
	wait.Wait()
}

func TestMultipleSnifers(t *testing.T) {
	wait := new(sync.WaitGroup)
	sn1 := NewSnifer("lo", &SniferConfig{
		Filter:    "",
		Timeout:   time.Duration(2) * time.Second,
		MaxPacket: 1600,
		Promod:    true,
		Offline:   false,
	})

	sn2 := NewSnifer("lxcbr0", &SniferConfig{
		Filter:    "",
		Timeout:   time.Duration(2) * time.Second,
		MaxPacket: 1600,
		Promod:    true,
		Offline:   false,
	})

	defer sn2.Kill()
	defer sn1.Kill()

	wait.Add(2)

	sn1.Listen(func(packet gopacket.Packet, sb *flux.Sub) {
		log.Println("Received Packet from sn1: ", packet)
		defer wait.Done()
		sb.Close()
	})

	sn2.Listen(func(packet gopacket.Packet, sb *flux.Sub) {
		log.Println("Received Packet from sn2: ", packet)
		// defer sb.Close()
		defer wait.Done()
		sb.Close()
	})

	err := sn1.Live()
	err2 := sn2.Live()
	checkTestError(t, err, "Unable to start snifer")
	checkTestError(t, err2, "Unable to start snifer")
	_ = sn1.Live()
	wait.Wait()
}
