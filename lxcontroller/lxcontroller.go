//Package lxcontroller handles the management of lxc containers with a safe and easy API
package lxcontroller

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	path "path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcapgo"

	"github.com/flynn/go-shlex"
	"github.com/honeycast/hoconf/api"
	"github.com/influx6/flux"
	"gopkg.in/lxc/go-lxc.v2"
)

var (
	//Debug is the defualt logger for lxcontrollers
	// Debug = log.New(os.Stdout, "Lxcontroller", log.Lshortfile)

	zero      = time.Duration(0)
	execess   = regexp.MustCompile(`^[\s|\n|\r|\t]+$`)
	execessmd = regexp.MustCompile(`^[\s\n\r\t]+$`)
	//ErrBadController defines a error when a controller is not properly inited
	ErrBadController = errors.New("Bad Contoller")
	//ErrBadBoot defines a error when a controller is not properly inited
	ErrBadBoot = errors.New("Bad Controller Boot Process")
	//ErrBadIP defines a error when a controller is not properly inited
	ErrBadIP = errors.New("IP NOT ACQUIRED")
	//ErrBadSnifer is for when the snifer is not fully ready yet
	ErrBadSnifer = errors.New("Container Snif Not Ready!")
	//splitter allows splitting of strings with quoted options
	stdErrBuffer bytes.Buffer
	stdOutBuffer bytes.Buffer
)

const (
	// THwardState = "THWARD"
	THwardState = "THWARD"
	// FreezingState = "FREEZING"
	FreezingState = "FREEZING"
	//FrozenState represents a frozen state
	FrozenState = "FROZEN"
	//AbortingState represents a aborting state
	AbortingState = "ABORTING"
	//RunningState represents a running state
	RunningState = "RUNNING"
	//StoppedState represents a stoped state
	StoppedState = "STOPPED"
	//StoppingState represents a stopping state
	StoppingState = "STOPPING"
	//StartingState presents a not starting state
	StartingState = "STARTING"
	//NotReadyState presents a not ready state
	NotReadyState = "NOTREADY"
)

type (

	//AttachMod allows modification of an attach command
	AttachMod func(lxc.AttachOptions) lxc.AttachOptions

	//ControlProfile defines the profile to use in the create method of the lxc
	ControlProfile struct {
		Path           string
		Create         *lxc.TemplateOptions
		Clone          *lxc.CloneOptions
		Attach         *lxc.AttachOptions
		Checks         *lxc.CheckpointOptions
		Restores       *lxc.RestoreOptions
		Configurations *flux.SecureMap
		Durations      *Durations
		Hooks          *api.Hooks
		HookFx         *HookFx
		DataFolder     string
		DeltaFolder    string
		Snifers        bool
		RetryMax       int
	}

	//HookFx provide state hooks for controllers
	HookFx struct {
		BeforeFreeze   func(Controllers)
		BeforeUnfreeze func(Controllers)
		BeforeBoot     func(Controllers)
		AfterBoot      func(Controllers)
		BeforeDrop     func(Controllers)
		AfterDrop      func(Controllers)
		AfterFreeze    func(Controllers)
		AfterUnfreeze  func(Controllers)
	}

	//Durations defines duration times for controllers
	Durations struct {
		Freeze time.Duration
		Retry  time.Duration
		Stop   time.Duration
	}

	//Controllers defines the interface method rules for lxc controller structs
	Controllers interface {
		Name() string
		Profile() *ControlProfile
		State() string
		NetworkInterfaceLink() string
		NetworkInterface() string
		NetworkInterfaces() []string
		NetworkInterfaceLinks() []string
		IP() (string, error)
		Container() *lxc.Container
		Dial() error
		Freeze() error
		Unfreeze() error
		DropFreezed() error
		DropUnfreezed() error
		Drop() error
		Restore() error
		CheckPoint() (string, error)
		Snifer() Snifers
		Clock()
		DumpPackets(func(string)) error
		OnDrop() flux.StackStreamers
		OnFreezed() flux.StackStreamers
		Unfreezed() flux.StackStreamers
		//commands with a single try step
		Command(cmd string) error
		Commands([]string, AttachMod) error
		CommandFile(string, AttachMod) error
		CommandMultiples([][]string, AttachMod) error
		CommandNative([]string, *lxc.AttachOptions, AttachMod) error
		//Commanders with a duration and retry count
		CommandEvery(cmd string, retry int, ms time.Duration, fx func())
		CommandsEvery(cmd []string, mx AttachMod, retry int, ms time.Duration, fx func())
		CommandFileEvery(cmds string, m AttachMod, retry int, ms time.Duration, fx func())
		CommandMultiplesEvery(cmds [][]string, m AttachMod, retry int, ms time.Duration, fx func())
		//Cloing funcs
		Clone(string) (Controllers, error)
		CloneWith(string, lxc.CloneOptions) (Controllers, error)
	}

	//BaseController provides a container struct for lxc containers
	BaseController struct {
		bname      string
		bip        string
		bcontainer *lxc.Container
		bprofile   *ControlProfile
		sniferConf *SniferConfig
		snifer     Snifers
		ticker     *flux.ResetTimer
		stopper    *flux.ResetTimer
		frozen     flux.StackStreamers
		unfrozen   flux.StackStreamers
		closed     flux.StackStreamers
	}
)

func checkError(e error, msg string) {
	if e != nil {
		log.Printf("Message: (%s) with Error: (%+v)", msg, e)
	} else {
		log.Printf("Message: (%s) with NoError", msg)
	}
}

func getState(state lxc.State) string {
	switch state {
	case lxc.FROZEN:
		return FrozenState
	case lxc.RUNNING:
		return RunningState
	case lxc.STARTING:
		return StartingState
	case lxc.ABORTING:
		return AbortingState
	case lxc.FREEZING:
		return FreezingState
	case lxc.THAWED:
		return THwardState
	case lxc.STOPPED:
		return StoppedState
	case lxc.STOPPING:
		return StoppingState
	default:
		return NotReadyState
	}
}

//NewProfile returns a new controlprofile
func NewProfile(containerPath string, tlp *lxc.TemplateOptions, clt *lxc.CloneOptions, d, s time.Duration, snifer bool) (p *ControlProfile) {
	p = &ControlProfile{
		Path:           containerPath,
		Create:         tlp,
		Clone:          clt,
		Snifers:        snifer,
		RetryMax:       10,
		Configurations: flux.NewSecureMap(),
		HookFx:         &HookFx{},
		Hooks:          &api.Hooks{},
		Durations: &Durations{
			Freeze: d,
			Stop:   s,
			Retry:  time.Duration(1) * time.Second,
		},
		Attach: &lxc.AttachOptions{
			ClearEnv: true,
		},
		Checks: &lxc.CheckpointOptions{
			Directory: "",
			Stop:      false,
			Verbose:   true,
		},
		Restores: &lxc.RestoreOptions{
			Directory: "",
			Verbose:   true,
		},
	}
	return
}

//BaseProfile returns a basic control profile
func BaseProfile(tlp *lxc.TemplateOptions, d, s time.Duration, sn bool) (p *ControlProfile) {
	p = NewProfile(lxc.DefaultConfigPath(), tlp, &lxc.CloneOptions{
		Backend:  lxc.Directory,
		KeepName: true,
		Snapshot: true,
	}, d, s, sn)
	return
}

//BaseFsProfile returns a basic control profile
func BaseFsProfile(fs lxc.BackendStore, tlp *lxc.TemplateOptions, d, s time.Duration, snifer bool) (p *ControlProfile) {
	p = NewProfile(lxc.DefaultConfigPath(), tlp, &lxc.CloneOptions{
		Backend:  fs,
		KeepName: true,
		Snapshot: true,
	}, d, s, snifer)
	return
}

//TemplateFSProfile returns basic control profile with a default template
func TemplateFSProfile(clfs, tmfs lxc.BackendStore, template, distro, release, arch string, sn bool, d, s time.Duration) *ControlProfile {
	return BaseFsProfile(clfs, &lxc.TemplateOptions{
		Template: template,
		Distro:   distro,
		Release:  release,
		Arch:     arch,
		Backend:  tmfs,
	}, d, s, sn)
}

//BaseAufsProfile returns a basic control profile
func BaseAufsProfile(tlp *lxc.TemplateOptions, d, s time.Duration, snifer bool) *ControlProfile {
	return BaseFsProfile(lxc.Aufs, tlp, d, s, snifer)
}

//AufsTemplateProfile returns basic control profile with a default template
func AufsTemplateProfile(bkend lxc.BackendStore, template, distro, release, arch string, sn bool, d, s time.Duration) *ControlProfile {
	return BaseAufsProfile(&lxc.TemplateOptions{
		Template: template,
		Distro:   distro,
		Release:  release,
		Arch:     arch,
		Backend:  bkend,
	}, d, s, sn)
}

//BaseOverlayfsProfile returns a basic control profile
func BaseOverlayfsProfile(tlp *lxc.TemplateOptions, d, s time.Duration, snifer bool) *ControlProfile {
	return BaseFsProfile(lxc.Overlayfs, tlp, d, s, snifer)
}

//OverlayfsProfile returns basic control profile with a default template
func OverlayfsProfile(bkend lxc.BackendStore, template, distro, release, arch string, sn bool, d, s time.Duration) *ControlProfile {
	return BaseOverlayfsProfile(&lxc.TemplateOptions{
		Template: template,
		Distro:   distro,
		Release:  release,
		Arch:     arch,
		Backend:  bkend,
	}, d, s, sn)
}

//TemplateProfile returns basic control profile with a default template
func TemplateProfile(bkend lxc.BackendStore, template, distro, release, arch string, sn bool, d, s time.Duration) *ControlProfile {
	return BaseProfile(&lxc.TemplateOptions{
		Template: template,
		Distro:   distro,
		Release:  release,
		Arch:     arch,
		Backend:  bkend,
	}, d, s, sn)
}

//HasController returns true/false if a container already exists with the given name
func HasController(name, path string) bool {
	cons := lxc.ContainerNames()
	for _, id := range cons {
		if id == name {
			return true
		}
	}
	return false
}

//NewController returns a new BaseController instance
func NewController(name string, profile *ControlProfile, sni *SniferConfig) (*BaseController, error) {

	if name == "" {
		panic(fmt.Sprintf("Lxcontroller.Error Container name can not be empty"))
	}

	c, err := lxc.NewContainer(name, profile.Path)

	if err != nil {
		checkError(err, fmt.Sprintf("Allocating Container %s", name))
	} else {
		checkError(err, fmt.Sprintf("Allocated Container %s", name))
	}

	if !c.Defined() {

		log.Printf("Container %s is not defined, will try and create", name)

		if profile.Create == nil {
			log.Printf("ControlProfile has no creation template options for %s", name)
			return nil, ErrBadController
		}

		log.Printf("Using profile %+v for creating container %s", profile.Create, name)

		err = c.Create(*profile.Create)

		if err != nil {
			checkError(err, fmt.Sprintf("Creating Container (%s) with call to .Create()", name))
			return nil, err
		}

	}

	c.SetVerbosity(lxc.Verbose)

	b := &BaseController{
		bname:      name,
		bip:        "",
		bcontainer: c,
		bprofile:   profile,
		sniferConf: sni,
		snifer:     NewSnifer("", sni),
		frozen:     flux.NewIdentityStream(),
		closed:     flux.NewIdentityStream(),
		unfrozen:   flux.NewIdentityStream(),
	}

	confs := b.Profile().Configurations

	confs.Each(func(v, k interface{}, _ func()) {
		ks, ok := k.(string)
		if ok {
			vs, ok := v.(string)
			if ok {
				checkError(c.SetConfigItem(ks, vs), fmt.Sprintf("Setting Container Config Key:(%s) Value:(%s)", ks, vs))
			}
		}
	})

	if b.Profile().Durations.Stop != zero {
		log.Printf("Controller %s setting up Stop Timer at duration of %+s", b.Name(), b.Profile().Durations.Stop)
		b.stopper = flux.NewResetTimer(func() {}, func() {
			b.Drop()
		}, b.Profile().Durations.Stop, false, false)
	}

	if b.Profile().Durations.Freeze != zero {
		log.Printf("Controller %s setting up Freeze Timer at duration of %+s", b.Name(), b.Profile().Durations.Freeze)
		b.ticker = flux.NewResetTimer(func() {
			log.Println("unfreezing it now!")
			b.Unfreeze()
		}, func() {
			log.Println("freezing it now!")
			b.Freeze()
		}, b.Profile().Durations.Freeze, false, false)
	}

	return b, err
}

//CloneController returns a new cloned BaseController instance
func CloneController(from, to string, profile *ControlProfile, conf *SniferConfig) (Controllers, error) {

	root, err := NewController(from, profile, conf)

	if err != nil {
		return nil, err
	}

	cl, err := root.Clone(to)

	return cl, err
}

//State returns the container state
func (b *BaseController) State() string {
	cl := b.Container().State()
	return getState(cl)
}

//Name returns the container name
func (b *BaseController) Name() string {
	return b.bname
}

//Profile returns the profile of  controller
func (b *BaseController) Profile() *ControlProfile {
	return b.bprofile
}

//Container returns the container of  controller
func (b *BaseController) Container() *lxc.Container {
	return b.bcontainer
}

//Clock resets the internal timer
func (b *BaseController) Clock() {
	if b.ticker != nil {
		b.ticker.Add()
	}
}

//OnDrop returns a stream that gets emitted to when the internal container gets stopped renews
func (b *BaseController) OnDrop() flux.StackStreamers {
	return b.closed
}

//OnFreezed returns a stream that gets emitted to when the internal timer renews
func (b *BaseController) OnFreezed() flux.StackStreamers {
	return b.frozen
}

//Unfreezed returns a stream that gets emitted to when the internal timer
func (b *BaseController) Unfreezed() flux.StackStreamers {
	return b.unfrozen
}

//Snifer returns the container snifer
func (b *BaseController) Snifer() Snifers {
	return b.snifer
}

//IP returns the container IP
func (b *BaseController) IP() (string, error) {
	checkError(nil, fmt.Sprintf("Retrieving IP from %s with virtual eth0", b.NetworkInterface()))

	// ips, ex := b.bcontainer.IPAddresses()
	ips, ex := b.bcontainer.IPAddress("eth0")

	checkError(ex, fmt.Sprintf("Retrieving IP from %s from ips %s", b.Name(), ips))

	if len(ips) <= 0 {
		return "", ErrBadIP
	}

	return ips[0], ex
}

//Clone clones the container and returns a new controller for that clone
func (b *BaseController) Clone(name string) (Controllers, error) {
	state := HasController(name, b.Profile().Path)
	log.Printf("Checking if (%s) controller already exists in (%s)? %v", name, b.Profile().Path, state)
	if !state {
		err := b.Container().Clone(name, *b.Profile().Clone)
		checkError(err, fmt.Sprintf("Cloning %s container to %s", b.Name(), name))

		if err != nil {
			return nil, err
		}
	}

	return NewController(name, b.Profile(), b.sniferConf)
}

//CloneWith clones the container and returns a new controller for that clone using a custom profile
func (b *BaseController) CloneWith(name string, prof lxc.CloneOptions) (Controllers, error) {
	state := HasController(name, b.Profile().Path)
	log.Printf("Checking if (%s) controller already exists in (%s)? %v", name, b.Profile().Path, state)
	if !state {
		err := b.Container().Clone(name, prof)
		checkError(err, fmt.Sprintf("Cloning %s container to %s", b.Name(), name))
		if err != nil {
			return nil, err
		}
	}
	return NewController(name, b.Profile(), b.sniferConf)
}

//Dial connects and initaties the container
func (b *BaseController) Dial() error {
	lxc.Acquire(b.Container())

	if b.Profile().HookFx.BeforeBoot != nil {
		b.Profile().HookFx.BeforeBoot(b)
	}

	log.Printf("Dialing/Booting Container (%s)", b.Name())

	con := b.Container()
	isunfrozen := false

	var err error

	switch con.State() {
	case lxc.FROZEN:
		err = con.Unfreeze()
		isunfrozen = true
		checkError(err, fmt.Sprintf("Unfreezing Container (%s)", b.Name()))
	case lxc.STOPPED:
		err = con.Start()
		checkError(err, fmt.Sprintf("Starting Container (%s)", b.Name()))
	case lxc.RUNNING:
		checkError(nil, fmt.Sprintf("Container (%s) in current State: (%+v)", b.Name(), con.State()))
		return nil
	default:
		checkError(nil, fmt.Sprintf("Container (%s) in current State: (%+v)", b.Name(), con.State()))
		err = ErrBadBoot
	}

	if err != nil {
		return err
	}

	if !con.Wait(lxc.RUNNING, 60) {
		checkError(ErrBadBoot, fmt.Sprintf("Wait Time Expired for Container (%s)", b.Name()))
		return ErrBadController
	}

	var dest string

	log.Printf("Begin IP Acquisition process for container (%s)", b.Name())
	for {
		ip, err := con.IPAddress("eth0")

		if err != nil {
			checkError(err, fmt.Sprintf("Waiting for Container (%s) IP to settle", b.Name()))
			time.Sleep(time.Millisecond * time.Duration(1000))
			continue
		}

		dest = ip[0]
		b.bip = dest
		break
	}

	log.Printf("Container (%s) IP Acquisition Succeed! IP Address: (%s)", b.Name(), dest)

	log.Printf("Setting Controller Snifer (%s) to (%s)!", b.Name(), b.NetworkInterface())
	b.snifer.SwitchSource(b.NetworkInterface())

	log.Printf("Will Execute Script for Start: %t or Unfrozen: %t for %s", !isunfrozen, isunfrozen, b.Name())

	b.turnOnSnifer()

	if isunfrozen {
		if b.Profile().Hooks.Unfreeze != "" {
			log.Printf("Running Controller Unfreeze script file: %s for %s", b.Profile().Hooks.Unfreeze, b.Name())
			b.CommandFileEvery(b.Profile().Hooks.Unfreeze, nil, b.Profile().RetryMax, b.Profile().Durations.Retry, func() {
				if b.Profile().HookFx.AfterBoot != nil {
					b.Profile().HookFx.AfterBoot(b)
				}
			})
		}
	} else if !isunfrozen {
		if b.Profile().Hooks.Boot != "" {
			log.Printf("Running Controller Startup file: %s for %s", b.Profile().Hooks.Boot, b.Name())
			b.CommandFileEvery(b.Profile().Hooks.Boot, nil, b.Profile().RetryMax, b.Profile().Durations.Retry, func() {
				if b.Profile().HookFx.AfterBoot != nil {
					b.Profile().HookFx.AfterBoot(b)
				}
			})
		}
	} else {
		if b.Profile().HookFx.AfterBoot != nil {
			b.Profile().HookFx.AfterBoot(b)
		}
	}

	log.Println("Starting Freeze/Unfreeze Ticker")
	b.ticker.Add()

	return nil
}

func (b *BaseController) turnOnSnifer() {
	if !b.Profile().Snifers {
		return
	}
	log.Printf("Switching On Controller Snifer (%s)!", b.Name())
	b.snifer.Live()
}

func (b *BaseController) turnOffSnifer() {
	if !b.Profile().Snifers {
		return
	}
	log.Printf("Switching off Controller Snifer (%s) !", b.Name())
	b.snifer.Kill()
}

//Drop disconnects and shutdowns the container
func (b *BaseController) Drop() error {
	if b.Profile().HookFx.BeforeDrop != nil {
		b.Profile().HookFx.BeforeDrop(b)
	}

	state := b.State()

	if b.Profile().Hooks.Shutdown != "" {
		log.Printf("Running Controller Shutdown script file: %s for %s", b.Profile().Hooks.Shutdown, b.Name())
		b.CommandFileEvery(b.Profile().Hooks.Shutdown, nil, b.Profile().RetryMax, b.Profile().Durations.Retry, func() {
			b.turnOffSnifer()
			log.Printf("Stopping Container (%s) in State: (%s)", b.Name(), state)
			b.Container().Stop()
			log.Printf("Container (%s) Stopped. State (%s)", b.Name(), b.State())
			lxc.Release(b.Container())
			log.Printf("Releasing Container (%s) ", b.Name())

			if b.Profile().HookFx.AfterDrop != nil {
				b.Profile().HookFx.AfterDrop(b)
			}
		})
	} else {
		b.turnOffSnifer()
		log.Printf("Stopping Container (%s) in State: (%s)", b.Name(), state)
		b.Container().Stop()
		log.Printf("Container (%s) Stopped. State (%s)", b.Name(), b.State())
		lxc.Release(b.Container())
		log.Printf("Releasing Container (%s) ", b.Name())
		if b.Profile().HookFx.AfterDrop != nil {
			b.Profile().HookFx.AfterDrop(b)
		}
	}

	if b.ticker != nil {
		b.ticker.Close()
		b.stopper.Close()
	}

	defer b.closed.Close()

	b.frozen.Close()
	b.unfrozen.Close()
	b.closed.Emit(b)
	b.snifer.Close()

	return nil
}

//DropFreezed disconnects and shutdowns the container
func (b *BaseController) DropFreezed() error {
	if b.Profile().HookFx.BeforeDrop != nil {
		b.Profile().HookFx.BeforeDrop(b)
	}

	if b.Profile().Hooks.Shutdown != "" {
		log.Printf("Running Controller Shutdown script file: %s for %s", b.Profile().Hooks.Shutdown, b.Name())
		b.CommandFileEvery(b.Profile().Hooks.Shutdown, nil, b.Profile().RetryMax, b.Profile().Durations.Retry, func() {
			b.turnOffSnifer()
			b.Freeze()
			lxc.Release(b.Container())
			defer b.closed.Close()
			b.closed.Emit(b)

			if b.Profile().HookFx.AfterDrop != nil {
				b.Profile().HookFx.AfterDrop(b)
			}
		})
	} else {
		b.turnOffSnifer()
		b.Freeze()
		lxc.Release(b.Container())
		defer b.closed.Close()
		b.closed.Emit(b)

		if b.Profile().HookFx.AfterDrop != nil {
			b.Profile().HookFx.AfterDrop(b)
		}
	}

	if b.ticker != nil {
		b.ticker.Close()
		b.stopper.Close()
	}

	b.snifer.Close()

	return nil
}

//DropUnfreezed disconnects and shutdowns the container
func (b *BaseController) DropUnfreezed() error {
	if b.Profile().HookFx.BeforeDrop != nil {
		b.Profile().HookFx.BeforeDrop(b)
	}

	state := b.State()

	if b.Profile().Hooks.Shutdown != "" {
		log.Printf("Running Controller Shutdown script file: %s for %s", b.Profile().Hooks.Shutdown, b.Name())
		b.CommandFileEvery(b.Profile().Hooks.Shutdown, nil, b.Profile().RetryMax, b.Profile().Durations.Retry, func() {
			b.turnOffSnifer()
			defer log.Printf("Releasing Container (%s) in State (%s)", b.Name(), state)
			lxc.Release(b.Container())
			defer log.Printf("Released Container (%s) in State (%s)", b.Name(), b.State())
			defer b.closed.Close()
			b.closed.Emit(b)

			if b.Profile().HookFx.AfterDrop != nil {
				b.Profile().HookFx.AfterDrop(b)
			}
		})
	} else {
		b.turnOffSnifer()
		defer log.Printf("Releasing Container (%s) in State (%s)", b.Name(), state)
		lxc.Release(b.Container())
		defer log.Printf("Released Container (%s) in State (%s)", b.Name(), b.State())
		defer b.closed.Close()
		b.closed.Emit(b)

		if b.Profile().HookFx.AfterDrop != nil {
			b.Profile().HookFx.AfterDrop(b)
		}
	}

	if b.ticker != nil {
		b.ticker.Close()
		b.stopper.Close()
	}
	b.snifer.Close()

	return nil
}

//Unfreeze unfreezes the container
func (b *BaseController) Unfreeze() error {
	log.Printf("lxcontroller.UnfreezeCommand: Checking Controller %s with State %s", b.Name(), b.State())

	if b.stopper != nil {
		b.stopper.Close()
	}

	if b.Profile().HookFx.BeforeUnfreeze != nil {
		b.Profile().HookFx.BeforeUnfreeze(b)
	}

	err := b.Dial()

	if err != nil {
		return err
	}

	log.Printf("lxcontroller.UnFrozenCommand:After: State of Controller %s with State %s", b.Name(), b.State())
	b.unfrozen.Emit(b)

	if b.Profile().HookFx.AfterUnfreeze != nil {
		b.Profile().HookFx.AfterUnfreeze(b)
	}

	return nil
}

//Freeze freezes the container
func (b *BaseController) Freeze() error {
	defer lxc.Release(b.Container())

	if b.stopper != nil {
		b.stopper.Add()
	}

	if b.Profile().HookFx.BeforeFreeze != nil {
		b.Profile().HookFx.BeforeFreeze(b)
	}

	state := b.State()

	if b.Profile().Hooks.Freeze != "" {
		log.Printf("Running Controller Freeze script file: %s for %s", b.Profile().Hooks.Freeze, b.Name())
		b.CommandFileEvery(b.Profile().Hooks.Freeze, nil, b.Profile().RetryMax, b.Profile().Durations.Retry, func() {
			b.turnOffSnifer()
			log.Printf("Freezing Container (%s) in State: (%s)", b.Name(), state)
			b.Container().Freeze()
			log.Printf("Container (%s) Frozen. State (%s)", b.Name(), b.State())
			b.frozen.Emit(b)

			if b.Profile().HookFx.AfterFreeze != nil {
				b.Profile().HookFx.AfterFreeze(b)
			}
		})
		return nil
	}

	b.turnOffSnifer()
	log.Printf("Freezing Container (%s) in State: (%s)", b.Name(), state)
	b.Container().Freeze()
	log.Printf("Container (%s) Frozen. State (%s)", b.Name(), b.State())
	b.frozen.Emit(b)

	if b.Profile().HookFx.AfterFreeze != nil {
		b.Profile().HookFx.AfterFreeze(b)
	}

	return nil
}

//NetworkInterfaceLink returns the network interface (bridge device) used
func (b *BaseController) NetworkInterfaceLink() string {
	links := b.NetworkInterfaceLinks()

	if links == nil {
		return ""
	}

	return links[0]
}

//NetworkInterfaceLinks returns the network interface (bridge device) used
func (b *BaseController) NetworkInterfaceLinks() []string {
	networks := b.Container().ConfigItem("lxc.network")
	var itype []string
	var itypes []string

	for ind := range networks {
		itypes = b.Container().RunningConfigItem(fmt.Sprintf("lxc.network.%d.type", ind))

		if itypes == nil {
			continue
		}

		if itypes[0] == "veth" {
			itype = b.Container().RunningConfigItem(fmt.Sprintf("lxc.network.%d.link", ind))
		}
	}

	return itype
}

//NetworkInterfaces returns the network interfaces (bridge/veth device) used
func (b *BaseController) NetworkInterfaces() []string {
	networks := b.Container().ConfigItem("lxc.network")
	var itype []string
	var itypes []string

	for ind := range networks {
		itypes = b.Container().RunningConfigItem(fmt.Sprintf("lxc.network.%d.type", ind))

		if itypes == nil {
			continue
		}

		if itypes[0] == "veth" {
			itype = b.Container().RunningConfigItem(fmt.Sprintf("lxc.network.%d.veth.pair", ind))
		} else {
			itype = b.Container().RunningConfigItem(fmt.Sprintf("lxc.network.%d.link", ind))
		}
	}

	return itype
}

//NetworkInterface returns the first network interface (bridge device) used
func (b *BaseController) NetworkInterface() string {
	isets := b.NetworkInterfaces()

	if isets == nil {
		return ""
	}

	return isets[0]
}

//CheckPoint performs the checkpointing operation of the
//controller
func (b *BaseController) CheckPoint() (string, error) {
	tmpdir := path.Clean(fmt.Sprintf("./%s/checkpoints/%s", b.Profile().DataFolder, b.Name()))

	err := os.MkdirAll(tmpdir, 0666)

	if err != nil && !os.IsExist(err) {
		return tmpdir, err
	}

	cx := *b.Profile().Checks
	cx.Directory = tmpdir

	err = b.Container().Checkpoint(cx)

	return tmpdir, err
}

//Restore performs the restoration operation of the controller
func (b *BaseController) Restore() error {
	tmpdir := path.Clean(fmt.Sprintf("./%s/checkpoints/%s", b.Profile().DataFolder, b.Name()))

	_, err := os.Stat(tmpdir)

	if os.IsNotExist(err) {
		return err
	}

	cx := *b.Profile().Restores
	cx.Directory = tmpdir

	return b.Container().Restore(cx)
}

//DumpPackets attaches a gopcap writer to a file for dumping packets coming from the
//snifer to a file and calls a function that is supplied when done(i.e frozen or drop)
func (b *BaseController) DumpPackets(fx func(string)) error {
	if b.State() != RunningState {
		return ErrBadController
	}

	dir := fmt.Sprintf("./%s/%s", b.Profile().DataFolder, "packets")

	err := os.MkdirAll(dir, 0666)

	if err != nil && !os.IsExist(err) {
		checkError(err, fmt.Sprintf("Creating dir %s for %s", dir, b.Name()))
	}

	fname := path.Clean(fmt.Sprintf("%s/%s.%d.pcap", dir, b.Name(), rand.Intn(3234)*20))
	dum, err := os.Create(fname)

	if err != nil {
		checkError(err, fmt.Sprintf("Creating file %s for %s", fname, b.Name()))
		return err
	}

	lt, err := b.snifer.LinkType()

	if err != nil {
		checkError(err, fmt.Sprintf("Retrieving Snifer linkType for %s", b.Name()))
		return err
	}

	gow := pcapgo.NewWriter(dum)

	if err := gow.WriteFileHeader(65536, lt); err != nil {
		checkError(err, fmt.Sprintf("Writing pcap Headers %s", b.Name()))
		return err
	}

	sub := b.Snifer().Listen(func(packet gopacket.Packet, _ flux.Stacks) {
		data := packet.Data()
		err := gow.WritePacket(gopacket.CaptureInfo{
			Timestamp:     time.Now(),
			CaptureLength: len(data),
			Length:        len(data),
		}, data)
		checkError(err, fmt.Sprintf("Dumping Packet Data to %s", fname))
	})

	end := new(sync.Once)
	ender := func() {
		defer sub.Close()
		dum.Close()
		log.Printf("Closing gopacket Writer and File for %s snifer on file %s", b.Name(), fname)
		if fx != nil {
			fx(fname)
		}
	}

	_ = b.OnFreezed().Stack(func(b interface{}, sz flux.Stacks) interface{} {
		defer sz.Close()
		end.Do(ender)
		return b
	}, true)

	_ = b.OnDrop().Stack(func(b interface{}, sz flux.Stacks) interface{} {
		defer sz.Close()
		end.Do(ender)
		return b
	}, true)

	return nil
}

//CommandNative runs a command using the lxc.controller.Command
func (b *BaseController) CommandNative(cmd []string, at *lxc.AttachOptions, fx AttachMod) error {
	if b.State() != RunningState {
		return ErrBadController
	}

	var ax lxc.AttachOptions

	if fx != nil {
		ax = fx(*at)
	} else {
		ax = *at
	}

	fsname := fmt.Sprintf("%s/%s.%d", b.Profile().DataFolder, "outerr", rand.Intn(400)*3)
	eobuff, err := os.Create(fsname)

	if err != nil {
		return err
	}

	defer eobuff.Close()
	defer os.Remove(fsname)

	ax.StdoutFd = eobuff.Fd()
	ax.StderrFd = eobuff.Fd()

	log.Printf("Lxcontoller.CommandNative.AttachOptions: Using attach: %+s using temp: %s", ax, eobuff.Name())
	_, err = b.Container().RunCommand(cmd, ax)

	if err != nil {
		checkError(err, fmt.Sprintf("Lxcontroller.CommandNative.Err for Cmd: (%+s)", cmd))
		return err
	}

	content, err := ioutil.ReadFile(fsname)

	if err != nil {
		return err
	}

	log.Printf("Lxcontroller.CommandNative::Stderr %+s for Cmd: (%+s) Result:(%+s)", eobuff.Name(), cmd, string(content))

	return nil
}

//Command runs a command on the container
func (b *BaseController) Command(cmds string) error {
	sl, err := shlex.Split(cmds)

	if err != nil {
		return err
	}

	return b.CommandNative(sl, b.Profile().Attach, nil)
}

//Commands runs a command on the container
func (b *BaseController) Commands(cmd []string, mx AttachMod) error {
	return b.CommandNative(cmd, b.Profile().Attach, mx)
}

//CommandEvery runs a command on the container
func (b *BaseController) CommandEvery(cmds string, retry int, ms time.Duration, fx func()) {
	max := 0
	go func() {
		for {

			if max >= retry {
				break
			}

			// time.Sleep(ms)
			err := b.Command(cmds)

			if err == nil {
				log.Printf("Exection of %s passed with no error from %s!", cmds, b.Name())
				break
			}

			checkError(err, fmt.Sprintf("Retrying Exection of File %s with Retry at %d of %d trials for %s", cmds, max, retry, b.Name()))
			max++
			time.Sleep(ms)
		}
		fx()
	}()
}

//CommandsEvery runs a command on the container
func (b *BaseController) CommandsEvery(cmds []string, mx AttachMod, retry int, ms time.Duration, fx func()) {
	max := 0
	go func() {
		for {

			if max >= retry {
				break
			}

			// time.Sleep(ms)
			err := b.Commands(cmds, mx)

			if err == nil {
				log.Printf("Exection of %s passed with no error from %s!", cmds, b.Name())
				break
			}

			checkError(err, fmt.Sprintf("Retrying Exection of File %s with Retry at %d of %d trials for %s", cmds, max, retry, b.Name()))
			max++
			time.Sleep(ms)
		}
		fx()
	}()
}

//CommandFileEvery runs a command on the container
func (b *BaseController) CommandFileEvery(cmds string, m AttachMod, retry int, ms time.Duration, fx func()) {

	max := 0
	go func() {
		for {

			if max >= retry {
				break
			}

			// time.Sleep(ms)
			err := b.CommandFile(cmds, m)

			if err == nil {
				log.Printf("Exection of %s passed with no error from %s!", cmds, b.Name())
				break
			}

			checkError(err, fmt.Sprintf("Retrying Exection of File %s with Retry at %d of %d trials for %s", cmds, max, retry, b.Name()))
			max++
			time.Sleep(ms)
		}
		fx()
	}()
}

//CommandMultiplesEvery runs a command on the container
func (b *BaseController) CommandMultiplesEvery(cmds [][]string, m AttachMod, retry int, ms time.Duration, fx func()) {
	max := 0
	go func() {
		for {
			if max >= retry {
				break
			}

			// time.Sleep(ms)
			err := b.CommandMultiples(cmds, m)

			if err == nil {
				log.Printf("Exection of %s passed with no error from %s!", cmds, b.Name())
				break
			}

			checkError(err, fmt.Sprintf("Retrying Exection of File %s with Retry at %d of %d trials for %s", cmds, max, retry, b.Name()))
			max++
			time.Sleep(ms)
		}
		fx()
	}()
}

//CommandMultiples runs a command on the container
func (b *BaseController) CommandMultiples(cmds [][]string, m AttachMod) error {
	for _, cmd := range cmds {
		err := b.Commands(cmd, m)
		if err != nil {
			return err
		}
	}
	return nil
}

//CommandFile runs a command on the container from a file whole content
func (b *BaseController) CommandFile(file string, mx AttachMod) error {
	if b.State() != RunningState {
		return ErrBadController
	}

	_, err := os.Stat(file)

	if os.IsNotExist(err) {
		return err
	}

	f, err := ioutil.ReadFile(file)

	if err != nil {
		return err
	}

	sl, err := shlex.Split(string(f))

	if err != nil {
		return err
	}

	return b.CommandNative(sl, b.Profile().Attach, mx)
}
