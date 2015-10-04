package api

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/honeycast/proxies"
	"gopkg.in/yaml.v2"
)

type (

	//Hooks represent files exection hooks of controller
	Hooks struct {
		//Freeze hook file path
		Freeze string `yaml:"freeze" json:"freeze"`
		//Shutdown hook file path
		Shutdown string `yaml:"shutdown" json:"shutdown"`
		//Boot hook file path
		Boot string `yaml:"boot" json:"boot"`
		//Unfreeze hook file path
		Unfreeze string `yaml:"unfreeze" json:"unfreeze"`
	}

	//Address represents address used in submission of data into api
	Address struct {
		SessionStart string `yaml:"session_start" json:"session_start"`
		SessionEnd   string `yaml:"session_end" json:"session_end"`
		Ping         string `yaml:"ping" json:"ping"`
		Session      string `yaml:"session" json:"session"`
		Packets      string `yaml:"packet" json:"packet"`
		Network      string `yaml:"network" json:"network"`
		Delta        string `yaml:"delta" json:"delta"`
		Checkpoint   string `yaml:"checkpoint" json:"checkpoint"`
		Proxies      string `yaml:"proxies" json:"proxies"`
		API          string `yaml:"api" json:"api"`
	}

	//Folders provide the folder directories of config
	Folders struct {
		//keys folder for ssh keys
		Keys string `yaml:"keys" json:"keys"`
		//Folder of storing checkpoints and deltas
		Data string `yaml:"data" json:"data"`
		//Root of where containers are stored
		Container string `yaml:"container" json:"container"`
		//Delta is the delta subroot of the container dir
		Delta       string `yaml:"delta" json:"delta"`
		DeltaFolder string `yaml:"-" json:"-"`
		Logs        string `yaml:"logs" json:"logs"`
		Profiles    string `yaml:"profiles" json:"profiles"`
	}

	//Durations provides detailed time.duration for config
	Durations struct {
		Dial      time.Duration
		Server    time.Duration
		Conn      time.Duration
		Snifers   time.Duration
		Apims     time.Duration
		Freeze    time.Duration
		Profiling time.Duration
		Ping      time.Duration
		Stop      time.Duration
		Age       time.Duration
		Check     time.Duration
	}

	//Options provides a struct for options
	Options struct {
		//Fstype dictates the internal fs config
		Fstype string `yaml:"fs_type" json:"fs_type"`
	}

	//ContainerOptions provides a means of providing container options
	ContainerOptions struct {
		//ConfigOptions define configs to be set in Controllers
		ConfigOptions map[string]string `yaml:"config_options" json:"config_options"`
		//Cloneoptions for clone
		CloneOptions *Options `yaml:"clone_options" json:"clone_options"`
		//TemplateOptions for clone's parent
		TemplateOptions *Options `yaml:"template_options,omitempty" json:"template_options,omitempty"`
	}

	//auxillary version of proxies.ProxyRequest
	proxyreq struct {
		Type     string `yaml:"type" json:"type"`
		Port     string `yaml:"port" json:"port"` //its a number,type int
		Addr     string `yaml:"addr" json:"addr"`
		CertFile string `yaml:"cert" json:"cert"`
		KeyFile  string `yaml:"key" json:"key"`
		From     string `yaml:"-" json:"-"`
	}

	proxmap map[string]*proxyreq

	//Lxconfig represents a standard lxc Configuration
	Lxconfig struct {
		Proxies proxies.Proxies `yaml:"proxies" json:"proxies"`
		//name of honeypot
		Name string `yaml:"name" json:"name"`
		//Auth token for api
		Token string `yaml:"token" json:"token"`
		//Version data for server
		Version string `yaml:"version" json:"version"`
		//addr of service 0.0.0.0
		Addr string `yaml:"addr" json:"addr"`
		//name of rsa key
		Rsa string `yaml:"rsa" json:"rsa"`
		//port of service 22
		Port int `yaml:"port" json:"port"`
		//Freeeze time in minutes
		Freeze int `yaml:"freeze_time" json:"freeze_time"`
		//Snifers control enabling of snifing
		Snifers bool `yaml:"snifers" json:"snifers"`
		//Streaming controls enabling streaming of packets
		Streaming bool `yaml:"streaming" json:"streaming"`
		//Folder dirs for the configuration
		Folders *Folders `yaml:"folders" json:"folders"`
		//Checkpointing controls the checkpointing process
		Checkpointing bool `yaml:"checkpointing" json:"checkpointing"`
		//Logging sets the logger to log to a file
		Logging bool `yaml:"logging" json:"logging"`
		//Profiling enables the profiling features
		Profiling bool `yaml:"profiling" json:"profiling"`
		//UseProxy controls proxy use in api
		UseProxy bool `yaml:"use_proxy" json:"use_proxy"`
		//Executables controls execution of hookfiles
		Executables bool `yaml:"executables" json:"executables"`
		//ContainerOptions provide container api specific settings
		Containers *ContainerOptions
		//APIAddress locations
		Address *Address `yaml:"address" json:"address"`
		//Files hook
		Hooks *Hooks `yaml:"hooks" json:"hooks"`
		//Dirs defines a list of directories to create
		Dirs []string `yaml:"dirs" json:"dirs,omitempty"`
		//Filter for use in snifconf network filter
		Filter string `yaml:"filter" json:"filter"`
		//Template name for cloning
		Template string `yaml:"template" json:"template"`
		//ommitables
		//time duration for dumping stack
		Maxage  int `yaml:"maxage,omitempty" json:"maxage,omitempty"`
		Checkms int `yaml:"checkms_ms,omitempty" json:"check_ms,omitempty"`
		Stopms  int `yaml:"stop_ms,omitempty" json:"stop_ms,omitempty"`
		Pingms  int `yaml:"ping_ms,omitempty" json:"ping_ms,omitempty"`
		Dumpms  int `yaml:"dump_ms,omitempty" json:"dump_ms,omitempty"`
		//ms in seconds for routeconfig
		Routems int `yaml:"route_ms,omitempty" json:"route_ms,omitempty"`
		//total ms in seconds for connection dailing(container dailing)
		Serverms int `yaml:"server_ms,omitempty" json:"server_ms,omitempty"`
		//ms in seconds for NewServerConn duration
		Connms int `yaml:"conn_ms,omitempty" json:"conn_ms,omitempty"`
		//ms in seconds for connection dailing(makedial)
		Dialms       int  `yaml:"dail_ms,omitempty" json:"dial_ms,omitempty"`
		ProfileDelay int  `yaml:"profile_delay,omitempty" json:"profile_delay,omitempty"`
		Promodo      bool `yaml:"promodo" json:"promodo"`
		//Offline dictates snifer conf offline setting
		Offline bool `yaml:"offline" json:"offline"`
		//Maxpacket dictates Snifer max packet conf
		MaxPacket int `yaml:"max_packet,omitempty" json:"max_packet,omitempty"`
		//batching count for packing packets
		Batchcount int `yaml:"batch_count,omitempty" json:"batch_ms,omitempty"`
		//ms in sec for snifer connection
		Sniferms int `yaml:"snifer_ms,omitempty" json:"snifer_ms,omitempty"`
		//total retry for submission by api
		Retry int `yaml:"retry,omitempty" json:"retry,omitempty"`
		//space for channels in api
		ChannelSpace int `yaml:"channel_space,omitempty" json:"channel_space,omitempty"`
		//delay used by api to send out batch
		APIDelay int `yaml:"api_delay,omitempty" json:"api_delay,omitempty"`
		//Durations provide a set duration for elements
		Durations *Durations `yaml:"-" json:"-"`
	}

	auxillaryconf struct {
		Proxies proxmap `yaml:"proxies" json:"proxies"`
		Name    string  `yaml:"name" json:"name"`
		Token   string  `yaml:"token" json:"token"`
		Freeze  string  `yaml:"freeze_time" json:"freeze_time"`
		Addr    string  `yaml:"addr" json:"addr"`
		//name of rsa key
		Rsa     string `yaml:"rsa" json:"rsa"`
		Version string `yaml:"version" json:"version"`
		Port    string `yaml:"port" json:"port"`
		//Snifers control enabling of snifing
		Snifers string `yaml:"snifers" json:"snifers"`
		//Checkpointing controls the checkpointing process
		Checkpointing string `yaml:"checkpointing" json:"checkpointing"`
		//Folder dirs for the configuration
		Folders *Folders `yaml:"folders" json:"folders"`
		//Streaming controls enabling streaming of packets
		Streaming string `yaml:"streaming" json:"streaming"`
		//Logging sets the logger to log to a file
		Logging string `yaml:"logging" json:"logging"`
		//Profiling enables the profiling features
		Profiling string `yaml:"profiling" json:"profiling"`
		//UseProxy controls proxy use in api
		UseProxy string `yaml:"use_proxy" json:"use_proxy"`
		//Executables controls execution of hookfiles
		Executables string `yaml:"executables" json:"executables"`
		//ContainerOptions provide container api specific settings
		Containers *ContainerOptions
		//APIAddress locations
		Address *Address `yaml:"address" json:"address"`
		//Files hook
		Hooks *Hooks `yaml:"hooks" json:"hooks"`
		//Dirs defines a list of directories to create
		Dirs []string `yaml:"dirs" json:"dirs"`
		//Filter for use in snifconf network filter
		Filter string `yaml:"filter" json:"filter"`
		//Template name for cloning
		Template string `yaml:"template" json:"template"`
		//ommitables
		//time duration for dumping stack
		Maxage       string `yaml:"maxage,omitempty" json:"maxage,omitempty"`
		Checkms      string `yaml:"checkms_ms,omitempty" json:"check_ms,omitempty"`
		ProfileDelay string `yaml:"profile_delay,omitempty" json:"profile_delay,omitempty"`
		Dumpms       string `yaml:"dump_ms,omitempty" json:"dump_ms,omitempty"`
		//ms in seconds for routeconfig
		Stopms  string `yaml:"stop_ms,omitempty" json:"stop_ms,omitempty"`
		Pingms  string `yaml:"ping_ms,omitempty" json:"ping_ms,omitempty"`
		Routems string `yaml:"route_ms,omitempty" json:"route_ms,omitempty"`
		//total ms in seconds for connection NewServerConn in makedial)
		Serverms string `yaml:"server_ms,omitempty" json:"server_ms,omitempty"`
		//ms in seconds for total makeDial duration
		Connms string `yaml:"conn_ms,omitempty" json:"conn_ms,omitempty"`
		//ms in seconds for connection makedial net.DialTimeout
		Dialms string `yaml:"dail_ms,omitempty" json:"dial_ms,omitempty"`
		//batching count for packing packets
		Batchcount string `yaml:"batch_count,omitempty" json:"batch_ms,omitempty"`
		Promodo    string `yaml:"promodo" json:"promodo"`
		//Offline dictates snifer conf offline setting
		Offline string `yaml:"offline" json:"offline"`
		//Maxpacket dictates snifer conf max packet setting
		MaxPacket string `yaml:"max_packet,omitempty" json:"max_packet,omitempty"`
		//ms in sec for snifer connection
		Sniferms string `yaml:"snifer_ms,omitempty" json:"snifer_ms,omitempty"`
		//total retry for submission by api
		Retry string `yaml:"retry,omitempty" json:"retry,omitempty"`
		//space for channels in api
		ChannelSpace string `yaml:"channel_space,omitempty" json:"channel_space,omitempty"`
		//delay used by api to send out batch
		APIDelay string `yaml:"api_delay,omitempty" json:"api_delay,omitempty"`
	}
)

//NewDurations returns a durations instance
func NewDurations(d, s, c, ss, a, f, p, pg, stp, ma, ck time.Duration) *Durations {
	return &Durations{
		Dial:      d,
		Server:    s,
		Conn:      c,
		Snifers:   ss,
		Apims:     a,
		Freeze:    f,
		Profiling: p,
		Ping:      pg,
		Stop:      stp,
		Age:       ma,
		Check:     ck,
	}
}

//LoadYAML loads configuration
func (l *Lxconfig) LoadYAML(file string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	if len(data) <= 0 {
		return ErrEmptyFile
	}

	return l.ParseYAML(data)
}

//LoadJSON loads configuration
func (l *Lxconfig) LoadJSON(file string) error {
	data, err := ioutil.ReadFile(file)

	if err != nil {
		return err
	}

	return l.ParseJSON(data)
}

//ParseJSON parses yaml configuration
func (l *Lxconfig) ParseJSON(pack []byte) error {
	// aux := new(auxillaryconf)
	err := json.Unmarshal(pack, l)

	if err != nil {
		return err
	}

	l.Freeze = useDefaultInt(l.Freeze, FREEZEMS)
	l.ProfileDelay = useDefaultInt(l.ProfileDelay, PDELAY)
	l.Dumpms = useDefaultInt(l.Dumpms, DUMPS)
	l.Connms = useDefaultInt(l.Connms, CONNMS)
	l.Serverms = useDefaultInt(l.Serverms, SERVERMS)
	l.Dialms = useDefaultInt(l.Dialms, DIALMS)
	l.Retry = useDefaultInt(l.Retry, RETRYCOUNT)
	l.Batchcount = useDefaultInt(l.Batchcount, BATCH)
	l.ChannelSpace = useDefaultInt(l.ChannelSpace, CHANNELS)
	l.Stopms = useDefaultInt(l.Stopms, STOPMS)
	l.Pingms = useDefaultInt(l.Pingms, PINGMS)
	l.Routems = useDefaultInt(l.Routems, RETRYCOUNT)
	l.Sniferms = useDefaultInt(l.Sniferms, SNIFERMS)
	l.APIDelay = useDefaultInt(l.APIDelay, APIDELAY)
	l.MaxPacket = useDefaultInt(l.MaxPacket, MAXPACKET)
	l.Maxage = useDefaultInt(l.Maxage, MAXAGE)
	l.Checkms = useDefaultInt(l.Checkms, CHECKMS)
	l.Promodo = true

	l.validateConfig()
	return nil
}

//ParseYAML parses yaml configuration
func (l *Lxconfig) ParseYAML(pack []byte) error {
	aux := new(auxillaryconf)
	err := yaml.Unmarshal(pack, aux)

	if err != nil {
		return err
	}

	return l.ParseAux(aux)
}

func (l *Lxconfig) validateConfig() {
	MustNotReport(l.Port, 0, "config.port can not be 0")
	MustNotReport(l.Rsa, "", "config.rsa can not be empty")
	MustNotReport(l.Addr, "", "config.addr can not be empty")
	MustNotReport(l.Template, "", "config.template can not be empty")
	MustNotReport(l.Version, "", "config.version can not be empty")

	if l.Folders == nil {
		l.Folders = &Folders{}
	}

	if l.Containers == nil {
		l.Containers = &ContainerOptions{}
	}

	if l.Address == nil {
		l.Address = &Address{}
	}

	if l.Hooks == nil {
		l.Hooks = &Hooks{}
	}

	fod := l.Folders

	MustNotReport(fod.Keys, "", "config.Folders.Keys must not be empty")
}

//ParseAux parses the yaml/json data into config
func (l *Lxconfig) ParseAux(aux *auxillaryconf) error {

	l.Proxies = make(proxies.Proxies)
	l.Rsa = aux.Rsa
	l.Name = aux.Name
	l.Token = aux.Token
	l.Dirs = aux.Dirs
	l.Addr = aux.Addr
	l.Version = aux.Version
	l.Hooks = aux.Hooks
	l.Address = aux.Address
	l.Filter = aux.Filter
	l.Template = aux.Template
	l.Containers = aux.Containers
	l.Folders = aux.Folders

	//numbers
	l.Freeze = convertToInt(aux.Freeze, FREEZEMS)
	l.Port = convertToInt(aux.Port, PORT)

	//boolers
	l.Profiling = convertToBool(aux.Profiling, false)
	l.Logging = convertToBool(aux.Logging, true)
	l.Streaming = convertToBool(aux.Streaming, true)
	l.Executables = convertToBool(aux.Executables, true)
	l.UseProxy = convertToBool(aux.UseProxy, true)
	l.Snifers = convertToBool(aux.Snifers, true)
	l.Promodo = convertToBool(aux.Promodo, false)
	l.Offline = convertToBool(aux.Offline, false)
	l.Checkpointing = convertToBool(aux.Checkpointing, true)

	//ommits
	l.ProfileDelay = useDefaultConvert(aux.ProfileDelay, PDELAY)
	l.Dumpms = useDefaultConvert(aux.Dumpms, DUMPS)
	l.Connms = useDefaultConvert(aux.Connms, CONNMS)
	l.Serverms = useDefaultConvert(aux.Serverms, SERVERMS)
	l.Dialms = useDefaultConvert(aux.Dialms, DIALMS)
	l.Retry = useDefaultConvert(aux.Retry, RETRYCOUNT)
	l.Batchcount = useDefaultConvert(aux.Batchcount, BATCH)
	l.ChannelSpace = useDefaultConvert(aux.ChannelSpace, CHANNELS)
	l.Pingms = useDefaultConvert(aux.Pingms, PINGMS)
	l.Stopms = useDefaultConvert(aux.Stopms, STOPMS)
	l.Routems = useDefaultConvert(aux.Routems, RETRYCOUNT)
	l.Sniferms = useDefaultConvert(aux.Sniferms, SNIFERMS)
	l.APIDelay = useDefaultConvert(aux.APIDelay, APIDELAY)
	l.MaxPacket = useDefaultConvert(aux.MaxPacket, MAXPACKET)
	l.Maxage = useDefaultConvert(aux.Maxage, MAXAGE)
	l.Checkms = useDefaultConvert(aux.Checkms, CHECKMS)

	proxs := aux.Proxies

	for k, v := range proxs {
		port := useDefaultConvert(v.Port, 80)
		l.Proxies[k] = &proxies.ProxyRequest{
			Type:     v.Type,
			Port:     port,
			Addr:     v.Addr,
			CertFile: v.CertFile,
			KeyFile:  v.KeyFile,
			From:     v.From,
		}
	}

	aux = nil

	l.validateConfig()

	return nil
}

//NewConfig returns a new config operation
func NewConfig() *Lxconfig {
	return &Lxconfig{}
}
