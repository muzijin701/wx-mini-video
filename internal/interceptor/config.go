package interceptor

import (
	"github.com/MuZiJin701/wx-mini-video/internal/config"
	"github.com/MuZiJin701/wx-mini-video/internal/miniprogram"
)

type InterceptorConfig struct {
	Version                  string
	ProxySetSystem           bool
	ProxyServerHostname      string
	ProxyServerPort          int
	ProxySkipInstallRootCert bool
	ProxyUpstreamProxy       string
	MiniProgramTarget        miniprogram.Target
	MiniProgramStore         *miniprogram.Store
}

func NewInterceptorSettings(c *config.Config, settings config.AppSettings, store *miniprogram.Store) *InterceptorConfig {
	return &InterceptorConfig{
		Version:                  c.Version,
		ProxySetSystem:           settings.ProxySystem,
		ProxyServerHostname:      settings.ProxyHostname,
		ProxyServerPort:          settings.ProxyPort,
		ProxySkipInstallRootCert: settings.ProxySkipRootCert,
		ProxyUpstreamProxy:       settings.ProxyUpstream,
		MiniProgramTarget:        settings.Target,
		MiniProgramStore:         store,
	}
}
