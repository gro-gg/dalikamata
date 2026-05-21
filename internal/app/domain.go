package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

type DomainApp struct {
	NATSHost       string
	NATSPort       int
	DataDir        string
	WithNATSServer bool
	logger         *slog.Logger
}

func NewDomainApp(logger *slog.Logger) *DomainApp {
	return &DomainApp{
		NATSHost: dalinats.DefaultHost,
		NATSPort: dalinats.DefaultPort,
		DataDir:  "./data/nats",
		logger:   logger.With("service", "domain"),
	}
}

func (a *DomainApp) Run(ctx context.Context) error {
	if a.WithNATSServer {
		natsServer := dalinats.NewServer()
		natsServer.Host = a.NATSHost
		natsServer.Port = a.NATSPort
		natsServer.DataDir = a.DataDir
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := natsServer.Start(ctx); err != nil {
				slog.Error("running NATS server", "error", err)
			}
		}()
		defer wg.Wait()
		if err := natsServer.WaitForStartup(); err != nil {
			return fmt.Errorf("waiting for NATS server: %w", err)
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

	repository := repo.NewMemory()
	svc := domain.NewDomainService(repository, a.logger)
	port := dalinats.NewPort(a.logger, svc, svc)
	return port.Run(ctx, js)
}
