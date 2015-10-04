package api

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/lxc/go-lxc.v2"
)

func mkdir(dir string) error {
	err := os.MkdirAll(dir, 0600)

	if os.IsExist(err) {
		return nil
	}

	return nil
}

//MustNotReport combines Report and MustNotBe together
func MustNotReport(v, k interface{}, msg string) {
	Report(MustNotBe(v, k), msg)
}

//Report panic if error is not nil with message
func Report(err error, msg string) {
	if err != nil {
		panic(fmt.Sprintf("Message: %s Error: %s", msg, err.Error()))
	}
}

//MustNotBe reports an error if values are equal
func MustNotBe(val, not interface{}) error {
	if val == not {
		return ErrEmpty
	}
	return nil
}

func convertToBool(target string, def bool) bool {
	fo, err := strconv.ParseBool(target)
	if err != nil {
		return def
	}
	return fo
}

func convertToInt(target string, def int) int {
	fo, err := strconv.Atoi(target)
	if err != nil {
		return def
	}
	return fo
}

func useDefaultInt(target int, def int) int {
	if target == 0 {
		return def
	}
	return target
}

func useDefaultConvert(target string, def int) int {
	val := convertToInt(target, def)
	if val == 0 {
		return def
	}
	return val
}

//GetFS returns the fstype provided by the api
func GetFS(fs string) lxc.BackendStore {
	switch fs {
	case "aufs":
		return lxc.Aufs
	case "overlayfs":
		return lxc.Overlayfs
	case "dir":
		return lxc.Directory
	case "lvm":
		return lxc.LVM
	case "zfs":
		return lxc.ZFS
	case "btrfs":
		return lxc.Btrfs
	case "loopback":
		return lxc.Loopback
	case "best":
		return lxc.Best
	}
	return lxc.Best
}

const (
	// DUMPS = 6 (time in min for delta)
	DUMPS = 6
	// DIALMS = 5 (time in seconds for net.Dialtimeout)
	DIALMS = 5
	// CONNMS = 10 (max total time in secs for makedial expire)
	CONNMS = 15
	// SERVERMS = 10 (max time in secs for NewServerConn expire)
	SERVERMS = 25
	// BATCH = 1000 (max packet size for buffers)
	BATCH = 1000
	// CHANNELS = 100 (max channel size)
	CHANNELS = 100
	// ROUTEMS = 10
	ROUTEMS = 100
	// PDELAY = 30
	PDELAY = 30
	// SNIFERMS = 10
	SNIFERMS = 10
	// RETRYCOUNT = 10
	RETRYCOUNT = 10
	// APIDELAY = 10
	APIDELAY = 1
	// MAXPACKET = 1600
	MAXPACKET = 1600
	//PORT = 22 (default port for connections)
	PORT = 22
	//FREEZEMS = 6 (the freeze time in minutes)
	FREEZEMS = 6
	// PINGMS = 1
	PINGMS = 3
	// STOPMS = 240
	STOPMS = 240
	// MAXAGE = 30 set in seconds
	MAXAGE = (60 * 6)
	// CHECKMS = 30 set in seconds
	CHECKMS = (60 * 2)
)

var (
	//ErrEmpty represent empty field values
	ErrEmpty = errors.New("Empty Field")
	//ErrEmptyFile is for when the data recieved is empty
	ErrEmptyFile = errors.New("Empty File Content")
	//ErrInvalidExt represent a wrong extension
	ErrInvalidExt = errors.New("Extension Invalid (not .json or .yaml)")
)
