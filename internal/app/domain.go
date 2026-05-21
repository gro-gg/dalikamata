package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

type DomainApp struct {
	NATSHost string
	NATSPort int
	logger   *slog.Logger
}

func NewDomainApp(logger *slog.Logger) *DomainApp {
	return &DomainApp{
		NATSHost: dalinats.DefaultHost,
		NATSPort: dalinats.DefaultPort,
		logger:   logger.With("service", "domain"),
	}
}

func (a *DomainApp) Run(ctx context.Context) error {
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
	port := dalinats.NewPort(a.logger, dalinats.WithGitEventHandler(svc), dalinats.WithPipelineEventHandler(svc))
	return port.Run(ctx, js)
}
