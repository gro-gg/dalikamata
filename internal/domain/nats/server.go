package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

type Server struct {
	Host               string
	Port               int
	DataDir            string
	NatsStartupTimeout time.Duration
	started            chan struct{}
	startErr           error
}

func NewServer() *Server {
	return &Server{
		Host:               "0.0.0.0",
		Port:               4222,
		DataDir:            "./data/nats",
		NatsStartupTimeout: time.Second,
		started:            make(chan struct{}),
	}
}

func (s *Server) Start(ctx context.Context) error {

	natsOpts := &server.Options{
		Host:      s.Host,
		Port:      s.Port,
		JetStream: true,
		StoreDir:  s.DataDir,
		NoSigs:    true,
	}

	ns, err := server.NewServer(natsOpts)
	if err != nil {
		s.startErr = fmt.Errorf("creating NATS server: %w", err)
		close(s.started)
		return s.startErr
	}

	ns.Start()
	defer func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	}()

	if !ns.ReadyForConnections(s.NatsStartupTimeout) {
		s.startErr = fmt.Errorf("ns startup timed out: %s", s.NatsStartupTimeout)
	}
	close(s.started)
	if s.startErr != nil {
		return s.startErr
	}

	<-ctx.Done()

	return nil
}

func (s *Server) WaitForStartup() error {
	<-s.started
	return s.startErr
}
