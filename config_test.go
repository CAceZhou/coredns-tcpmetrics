package tcpmetrics

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestParseConfig(t *testing.T) {
	c := caddy.NewTestController("dns", `tcpmetrics 127.0.0.1:9443 {
        token 0123456789abcdef
        tls node.crt node.key
        sample 2s
        retain 10s
    }`)
	cfg, err := parse(c)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9443" || cfg.SampleInterval.String() != "2s" || cfg.Retention.String() != "10s" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestParseConfigRequiresStrongToken(t *testing.T) {
	c := caddy.NewTestController("dns", `tcpmetrics {
        token short
        tls node.crt node.key
    }`)
	if _, err := parse(c); err == nil {
		t.Fatal("short token accepted")
	}
}
