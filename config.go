package tcpmetrics

import (
	"fmt"
	"net"
	"time"

	"github.com/coredns/caddy"
)

type Config struct {
	Listen         string
	Token          string
	CertFile       string
	KeyFile        string
	SampleInterval time.Duration
	Retention      time.Duration
	AllowNonRoot   bool
}

func defaultConfig() Config {
	return Config{Listen: "127.0.0.1:9165", SampleInterval: 5 * time.Second, Retention: 30 * time.Second}
}

func parse(c *caddy.Controller) (Config, error) {
	cfg := defaultConfig()
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) > 1 {
			return cfg, c.ArgErr()
		}
		if len(args) == 1 {
			cfg.Listen = args[0]
		}
		for c.NextBlock() {
			property := c.Val()
			args = c.RemainingArgs()
			switch property {
			case "token":
				if len(args) != 1 {
					return cfg, c.ArgErr()
				}
				cfg.Token = args[0]
			case "tls":
				if len(args) != 2 {
					return cfg, c.ArgErr()
				}
				cfg.CertFile = args[0]
				cfg.KeyFile = args[1]
			case "sample":
				d, e := oneDuration(args)
				if e != nil {
					return cfg, e
				}
				cfg.SampleInterval = d
			case "retain":
				d, e := oneDuration(args)
				if e != nil {
					return cfg, e
				}
				cfg.Retention = d
			case "allow_non_root":
				if len(args) != 0 {
					return cfg, c.ArgErr()
				}
				cfg.AllowNonRoot = true
			default:
				return cfg, c.Errf("unknown tcpmetrics property %q", property)
			}
		}
	}
	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		return cfg, fmt.Errorf("invalid listen address: %w", err)
	}
	if len(cfg.Token) < 16 {
		return cfg, fmt.Errorf("token must contain at least 16 bytes")
	}
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return cfg, fmt.Errorf("tls certificate and key are required")
	}
	if cfg.SampleInterval <= 0 || cfg.Retention < cfg.SampleInterval {
		return cfg, fmt.Errorf("sample must be positive and retain must be at least sample")
	}
	return cfg, nil
}

func oneDuration(args []string) (time.Duration, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("expected one duration argument")
	}
	d, err := time.ParseDuration(args[0])
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", args[0], err)
	}
	return d, nil
}
