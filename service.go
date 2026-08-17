package tcpmetrics

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/CAceZhou/coredns-tcpmetrics/telemetry"
	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

type Service struct {
	Next     plugin.Handler
	cfg      Config
	store    *telemetry.Store
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
}

func New(cfg Config) *Service { return &Service{cfg: cfg} }

func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}
	if _, err := tls.LoadX509KeyPair(s.cfg.CertFile, s.cfg.KeyFile); err != nil {
		return fmt.Errorf("load tcpmetrics TLS keypair: %w", err)
	}
	provider, err := telemetry.NewSystemProvider(s.cfg.AllowNonRoot)
	if err != nil {
		return err
	}
	store := telemetry.NewStore(provider, s.cfg.SampleInterval, s.cfg.Retention)
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen tcpmetrics API: %w", err)
	}
	s.store = store
	s.listener = ln
	s.server = &http.Server{Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	store.Start(context.Background())
	telemetry.SetDefaultStore(store)
	go func() {
		if err := s.server.ServeTLS(ln, s.cfg.CertFile, s.cfg.KeyFile); err != nil && err != http.ErrServerClosed {
			log.Errorf("HTTPS API stopped: %v", err)
		}
	}()
	return nil
}

func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.server.Shutdown(ctx)
	s.store.Stop()
	telemetry.ClearDefaultStore(s.store)
	s.server = nil
	s.listener = nil
	return err
}

func (s *Service) Store() *telemetry.Store { return s.store }

func (s *Service) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(name, s.Next, ctx, w, r)
}
func (*Service) Name() string { return name }
