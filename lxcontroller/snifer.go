package lxcontroller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/influx6/flux"
)

//Snifers provide a means of gathering packets data from network device using gopackets
//it provides a very simple API ontop of a basic PacketSource handle

type (

	//SniferMod defines a modification function that takes a packet
	SniferMod func(gopacket.Packet) gopacket.Packet

	//Snifers defines the interface method rules for network snifters
	Snifers interface {
		Live() error
		Kill() error
		Close() error
		Source() string
		IsActive() bool
		SwitchSource(string)
		RetrieveSrcIP(gopacket.Packet) (net.IP, error)
		RetrieveDestIP(gopacket.Packet) (net.IP, error)
		WhenSrcAndDestIP(string, string) flux.StackStreamers
		WhenDestIP(string) flux.StackStreamers
		WhenSrcIP(string) flux.StackStreamers
		WhenBy(SniferMod) flux.StackStreamers
		Listen(func(gopacket.Packet, flux.Stacks)) flux.Stacks
		LinkType() (layers.LinkType, error)
	}

	//Snifer represent a instance of type Snifers that reads and allows corresponding
	//matching of packets read either from pcap files or live streams
	Snifer struct {
		source  string
		conf    *SniferConfig
		handle  *pcap.Handle
		psource *gopacket.PacketSource
		streams flux.StackStreamers
		active  int64
		killed  int64
	}

	//SniferConfig represents a standard config used by the gopacket source initialization
	SniferConfig struct {
		Filter    string
		Timeout   time.Duration
		MaxPacket int32
		Promod    bool
		Offline   bool
	}
)

var (
	//ErrBadPacket represents a packet no matching gopacket.Layer
	ErrBadPacket = errors.New("Recieved Bad gopacket.Packet")
	//ErrBadHandle represents a incorrectly setup snifer handle
	ErrBadHandle = errors.New("Snifer has no packet handle")
)

//DefaultConfig returns a standard default configuration
func DefaultConfig() *SniferConfig {
	return &SniferConfig{
		"eth0",
		time.Duration(1) * time.Minute,
		1600,
		true,
		false,
	}
}

//WhenSrcAndDestIP returns a flux.StreamInterface that contains packets that
//match the ip of the destination
func (s *Snifer) WhenSrcAndDestIP(src string, dest string) flux.StackStreamers {
	return s.WhenBy(func(packet gopacket.Packet) gopacket.Packet {
		iplayers := packet.Layer(layers.LayerTypeIPv4)

		if iplayers != nil {

			ipc, _ := iplayers.(*layers.IPv4)
			log.Printf("Emiting Packet Snif for SrcIP (%s) And DestIP (%s)", ipc.SrcIP.String(), ipc.DstIP.String())

			if ipc.DstIP.Equal(net.ParseIP(dest)) {
				if ipc.SrcIP.Equal(net.ParseIP(src)) {
					return packet
				}
			}
		}

		return nil
	})
}

//RetrieveSrcIP retrieves the src ip of a packet
func RetrieveSrcIP(packet gopacket.Packet) (net.IP, error) {
	iplayers := packet.Layer(layers.LayerTypeIPv4)

	if iplayers != nil {
		ipc, ok := iplayers.(*layers.IPv4)

		if ok {
			return ipc.SrcIP, nil
		}
	}
	return nil, ErrBadPacket
}

//RetrieveSrcPort retrieves the src port of a packet
func RetrieveSrcPort(packet gopacket.Packet) (int, error) {
	iplayers := packet.Layer(layers.LayerTypeTCP)

	if iplayers != nil {
		ipc, ok := iplayers.(*layers.TCP)

		if ok {
			return int(ipc.SrcPort), nil
		}
	}
	return 0, ErrBadPacket
}

//RetrieveDestPort retrieves the destination ip of a packet
func RetrieveDestPort(packet gopacket.Packet) (int, error) {
	iplayers := packet.Layer(layers.LayerTypeTCP)

	if iplayers != nil {
		ipc, ok := iplayers.(*layers.TCP)

		if ok {
			return int(ipc.DstPort), nil
		}

	}
	return 0, ErrBadPacket
}

//RetrieveDestIP retrieves the destination ip of a packet
func RetrieveDestIP(packet gopacket.Packet) (net.IP, error) {
	iplayers := packet.Layer(layers.LayerTypeIPv4)

	if iplayers != nil {
		ipc, ok := iplayers.(*layers.IPv4)

		if ok {
			return ipc.DstIP, nil
		}

	}
	return nil, ErrBadPacket
}

//RetrieveDestIP retrieves the destination ip of a packet
func (s *Snifer) RetrieveDestIP(packet gopacket.Packet) (net.IP, error) {
	return RetrieveDestIP(packet)
}

//RetrieveSrcIP retrieves the src ip of a packet
func (s *Snifer) RetrieveSrcIP(packet gopacket.Packet) (net.IP, error) {
	return RetrieveSrcIP(packet)
}

//WhenDestIP returns a flux.StreamInterface that contains packets that
//match the ip of the destination
func (s *Snifer) WhenDestIP(ip string) flux.StackStreamers {
	return s.WhenBy(func(packet gopacket.Packet) gopacket.Packet {
		ipc, err := s.RetrieveDestIP(packet)

		if err != nil {
			checkError(err, fmt.Sprintf("Checking Packet Dest IP against %s", ip))
			return nil
		}

		log.Printf("Checking Packet Snif for DestIP (%s) with (%s)", ipc.String(), ip)

		if ipc.Equal(net.ParseIP(ip)) {
			log.Printf("Emiting Packet Snif for DestIP (%s)", ipc.String())
			return packet
		}

		return nil
	})
}

//WhenSrcIP returns a flux.StreamInterface that contains packets that
//match the ip of the src
func (s *Snifer) WhenSrcIP(ip string) flux.StackStreamers {
	return s.WhenBy(func(packet gopacket.Packet) gopacket.Packet {
		ipc, err := s.RetrieveSrcIP(packet)

		if err != nil {
			checkError(err, fmt.Sprintf("Checking Packet Dest IP against %s", ip))
			return nil
		}

		log.Printf("Checking Packet Snif for SrcIP (%s) with (%s)", ipc.String(), ip)

		if ipc.Equal(net.ParseIP(ip)) {
			log.Printf("Emiting Packet Snif for SrcIP (%s)", ipc.String())
			return packet
		}

		return nil
	})
}

//Listen lets you subscribe into the stream
func (s *Snifer) Listen(fx func(gopacket.Packet, flux.Stacks)) flux.Stacks {
	return s.streams.Stack(func(p interface{}, sb flux.Stacks) interface{} {
		pc, ok := p.(gopacket.Packet)
		if ok {
			fx(pc, sb)
		}
		return p
	}, true)
}

//WhenBy takes a function that recieves a packet and performs an op and sends it to the return stream if it validates it so
func (s *Snifer) WhenBy(fx SniferMod) flux.StackStreamers {
	sx := flux.NewIdentityStream()

	_ = s.streams.Stack(func(data interface{}, _ flux.Stacks) interface{} {
		pc, ok := data.(gopacket.Packet)

		log.Printf("Checking Data if gopacket.Packet? Res: %v", ok)

		if ok {
			sx.Mux(fx(pc))
		}

		return data
	}, true)

	return sx
}

//NewSnifer returns a new basic snifter
func NewSnifer(source string, conf *SniferConfig) (sf *Snifer) {
	sf = &Snifer{
		source:  source,
		conf:    conf,
		streams: flux.NewIdentityStream(),
		active:  0,
		killed:  0,
	}

	return
}

//SwitchSource changes the snifer source string
func (s *Snifer) SwitchSource(bs string) {
	s.source = bs
}

//init re-initializes a snifer for uses
func (s *Snifer) init() error {
	var handle *pcap.Handle
	var err error

	if s.conf.Offline {
		handle, err = pcap.OpenOffline(s.source)
		checkError(err, fmt.Sprintf("Create offline handle %s", s.source))
	} else {
		handle, err = pcap.OpenLive(s.source, s.conf.MaxPacket, s.conf.Promod, s.conf.Timeout)
		checkError(err, fmt.Sprintf("Create Live handle %s", s.source))

		if err == nil {
			err = handle.SetBPFFilter(s.conf.Filter)
			checkError(err, fmt.Sprintf("Setting BPFFilter %s: %s", s.source, s.conf.Filter))
		}

	}

	if err != nil {
		checkError(err, fmt.Sprintf("Creating Snifer for %s", s.source))
		return err
	}

	s.handle = handle
	log.Printf("Snifer: Handler created and ready!")

	return nil
}

//IsActive returns true/false if its active
func (s *Snifer) IsActive() bool {
	bit := atomic.LoadInt64(&s.active)
	return int(bit) == 1
}

//Source returns the interface device/file being sniffed
func (s *Snifer) Source() string {
	return s.source
}

//Close turns on the snifer packet collection
func (s *Snifer) Close() error {
	s.streams.Close()
	return s.Kill()
}

//Kill turns on the snifer packet collection
func (s *Snifer) Kill() error {
	if s.IsActive() {
		atomic.StoreInt64(&s.killed, 1)
	}
	return nil
}

//LinkType returns the type of link of the handle
func (s *Snifer) LinkType() (layers.LinkType, error) {
	if s.handle == nil {
		return layers.LinkTypeNull, ErrBadHandle
	}
	return s.handle.LinkType(), nil
}

//Live turns on the snifer packet collection
func (s *Snifer) Live() error {
	if s.IsActive() {
		log.Println("Snifer is Still Active!")
		return nil
	}

	err := s.init()

	if err != nil {
		return err
	}

	if s.handle == nil {
		return ErrBadHandle
	}

	defer func() {
		err := recover()
		if err != nil {
			log.Printf("Panic Occured: %+s \n %+s", err, debug.Stack())
			return
		}
	}()

	log.Printf("Snifer: Init.... Coming Live for %s", s.source)
	s.psource = gopacket.NewPacketSource(s.handle, s.handle.LinkType())
	log.Printf("Snifer: Created PacketSource for %s", s.source)

	go func() {
		defer func() {
			err := recover()
			if err != nil {
				log.Printf("Panic Occured: %+s", err)
			}
		}()

		for {
			//experiemental anti-panic ,not sure if it will work,test didnt show
			//success
			defer func() {
				err := recover()
				if err != nil {
					log.Printf("Panic Occured: %+s", err)
				}
			}()

			if int(atomic.LoadInt64(&s.killed)) == 1 {
				atomic.StoreInt64(&s.active, 0)
				atomic.StoreInt64(&s.killed, 0)
				break
			}

			packet, err := s.psource.NextPacket()

			if err == io.EOF {
				atomic.StoreInt64(&s.active, 0)
				atomic.StoreInt64(&s.killed, 0)
				break
			}

			if err != nil {
				continue
			}

			s.streams.Emit(packet)
			//dont starve the cpu

		}

		s.Kill()
	}()

	atomic.StoreInt64(&s.active, 1)
	return nil
}
