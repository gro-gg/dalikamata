package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
)

type DomainApp struct {
	NATSHost           string
	NATSPort           int
	NatsStartupTimeout time.Duration
	DataDir            string
	WithNATSServer     bool
	GracePeriod        time.Duration
}

func NewDomainApp() *DomainApp {
	return &DomainApp{
		NATSHost:           dalinats.DefaultHost,
		NATSPort:           dalinats.DefaultPort,
		NatsStartupTimeout: 3 * time.Second,
		DataDir:            "./data/nats",
		WithNATSServer:     true,
		GracePeriod:        10 * time.Second,
	}
}

func (a *DomainApp) Run(ctx context.Context, logger *slog.Logger) error {
	var cancelNATS func()
	var err error

	if a.WithNATSServer {
		server := dalinats.NewServer()
		server.Host = a.NATSHost
		server.Port = a.NATSPort
		server.DataDir = a.DataDir
		cancelNATS, err = server.Start()
		if err != nil {
			return fmt.Errorf("start NATS server: %w", err)
		}
	}

	nc, err := nats.Connect(dalinats.NATSConnectionString(a.NATSHost, a.NATSPort))
	if err != nil {
		return fmt.Errorf("connecting to NATS server: %w", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("creating jetstream: %w", err)
	}

	domain := dalinats.NewPort(logger.With("service", "domain"))
	shutdownDomain, err := domain.Run(ctx, js)
	if err != nil {
		return fmt.Errorf("starting domain port: %w", err)
	}

	<-ctx.Done()

	shutdownFinished := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Go(func() {
		shutdownDomain()
	})
	if a.WithNATSServer {
		wg.Go(func() {
			cancelNATS()
		})
	}
	go func() {
		wg.Wait()
		close(shutdownFinished)
	}()

	select {
	case <-shutdownFinished:
		return nil
	case <-time.After(a.GracePeriod):
		return fmt.Errorf("shutdown not finished after graceperiod: %d", a.GracePeriod)
	}
}
