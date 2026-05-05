package nats

import (
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

type Server struct {
	Host               string
	Port               int
	DataDir            string
	NatsStartupTimeout time.Duration
}

func NewServer() *Server {
	return &Server{
		Host:               "0.0.0.0",
		Port:               4222,
		DataDir:            "./data/nats",
		NatsStartupTimeout: time.Second,
	}
}

func (s *Server) Start() (func(), error) {

	natsOpts := &server.Options{
		Host:      s.Host,
		Port:      s.Port,
		JetStream: true,
		StoreDir:  s.DataDir,
		NoSigs:    true,
	}

	ns, err := server.NewServer(natsOpts)
	if err != nil {
		return nil, fmt.Errorf("creating NATS server: %w", err)
	}

	go ns.Start()
	if !ns.ReadyForConnections(s.NatsStartupTimeout) {
		return nil, fmt.Errorf("ns startup timed out: %s", s.NatsStartupTimeout)
	}

	return func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	}, nil
}
