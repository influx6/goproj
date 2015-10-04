package serverconf

import (
	"github.com/goproj/hoconf/api"
	"github.com/goproj/lxcontroller"
	"github.com/goproj/proxies"
	"github.com/goproj/servicedrop"
)

type (

	//Fx provides functions set for specific operations
	Fx struct {
		Directors func(lxcontroller.Controllers, proxies.Factory)
	}

	//ServerConfig defines specific server connection details used by the service drop sshprotocol
	ServerConfig struct {
		Profile   *lxcontroller.ControlProfile
		Snifers   *lxcontroller.SniferConfig
		Route     *servicedrop.RouteConfig
		Meta      *api.Lxconfig
		Durations *api.Durations
		Hooks     *Fx
	}
)

//BaseServer returns a new ServerConfig instance
func BaseServer() *ServerConfig {
	return &ServerConfig{
		Meta:  api.NewConfig(),
		Hooks: &Fx{},
	}
}

//NewServer returns a new ServerConfig instance
func NewServer(d *api.Durations, m *api.Lxconfig, p *lxcontroller.ControlProfile, s *lxcontroller.SniferConfig, r *servicedrop.RouteConfig) *ServerConfig {
	return &ServerConfig{
		Profile:   p,
		Snifers:   s,
		Route:     r,
		Meta:      m,
		Durations: d,
		Hooks:     &Fx{},
	}
}
