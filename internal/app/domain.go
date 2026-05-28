package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

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
	nc, js, closeConn, err := nats.Connect(ctx, nats.NATSConnectionString(a.NATSHost, a.NATSPort), a.logger)
	if err != nil {
		return fmt.Errorf("connecting to NATS server: %w", err)
	}
	defer closeConn()

	repository := repo.NewMemory()
	svc := domain.NewDomainService(repository, repository, a.logger)

	ingestPort := dalinats.NewPort(a.logger, dalinats.WithGitEventHandler(svc), dalinats.WithCicdEventHandler(svc))
	queryPort := dalinats.NewQueryPort(a.logger, svc)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); errCh <- ingestPort.Run(ctx, js) }()
	go func() { defer wg.Done(); errCh <- queryPort.Run(ctx, nc) }()

	<-ctx.Done()
	wg.Wait()
	close(errCh)

	var joined error
	for e := range errCh {
		if e != nil && !errors.Is(e, context.Canceled) {
			joined = errors.Join(joined, e)
		}
	}
	return joined
}
