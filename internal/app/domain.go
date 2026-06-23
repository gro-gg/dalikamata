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
	// DBPath is the SQLite database file for persistent storage. When empty the
	// domain uses an in-memory repository (data lost on restart).
	DBPath string
	logger *slog.Logger
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

	var store domain.Repository
	var queryStore domain.QueryRepository
	if a.DBPath != "" {
		sqliteRepo, err := repo.NewSQLite(a.DBPath)
		if err != nil {
			return fmt.Errorf("opening sqlite repository: %w", err)
		}
		defer func() {
			if cerr := sqliteRepo.Close(); cerr != nil {
				a.logger.Error("closing sqlite repository", "error", cerr)
			}
		}()
		a.logger.Info("using persistent sqlite repository", "path", a.DBPath)
		store, queryStore = sqliteRepo, sqliteRepo
	} else {
		memoryRepo := repo.NewMemory()
		a.logger.Info("using in-memory repository (data not persisted)")
		store, queryStore = memoryRepo, memoryRepo
	}
	svc := domain.NewDomainService(store, queryStore, a.logger)

	ingestPort, err := dalinats.NewPort(a.logger,
		dalinats.WithGitEventHandler(svc),
		dalinats.WithCicdEventHandler(svc),
		dalinats.WithPlatformEventHandler(svc),
	)
	if err != nil {
		return fmt.Errorf("creating ingest port: %w", err)
	}
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
