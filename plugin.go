// Package tcpmetrics implements authenticated TCP socket telemetry for CoreDNS.
package tcpmetrics

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

const name = "tcpmetrics"

var log = clog.NewWithPlugin(name)

func init() { plugin.Register(name, setup) }

func setup(c *caddy.Controller) error {
	cfg, err := parse(c)
	if err != nil {
		return plugin.Error(name, err)
	}
	svc := New(cfg)
	c.OnStartup(svc.Start)
	c.OnShutdown(svc.Stop)
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler { svc.Next = next; return svc })
	return nil
}
