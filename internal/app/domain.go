package app

import (
	"context"
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
	"codeberg.org/aeforged/dalikamata/internal/nats"
)

type DomainApp struct {
	NATSHost string
	NATSPort int
	logger   *slog.Logger
}

func NewDomainApp(logger *slog.Logger) *DomainApp {
	return &DomainApp{
		NATSHost: nats.DefaultHost,
		NATSPort: nats.DefaultPort,
		logger:   logger.With("service", "domain"),
	}
}

func (a *DomainApp) Run(ctx context.Context) error {
	_, js, closeConn, err := nats.Connect(ctx, nats.NATSConnectionString(a.NATSHost, a.NATSPort), a.logger)
	if err != nil {
		return fmt.Errorf("connecting to NATS server: %w", err)
	}
	defer closeConn()

	repository := repo.NewMemory()
	svc := domain.NewDomainService(repository, repository, a.logger)
	port := dalinats.NewPort(a.logger, dalinats.WithGitEventHandler(svc), dalinats.WithCicdEventHandler(svc))
	return port.Run(ctx, js)
}
