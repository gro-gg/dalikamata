package app

import (
	"context"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/nats"
)

type NATSApp struct {
	Host    string
	Port    int
	DataDir string
	logger  *slog.Logger
	srv     *nats.Server
}

func NewNATSApp(logger *slog.Logger) *NATSApp {
	return &NATSApp{
		Host:    nats.DefaultHost,
		Port:    nats.DefaultPort,
		DataDir: "./data/nats",
		logger:  logger.With("service", "nats"),
		srv:     nats.NewServer(),
	}
}

func (a *NATSApp) Run(ctx context.Context) error {
	a.srv.Host = a.Host
	a.srv.Port = a.Port
	a.srv.DataDir = a.DataDir
	a.logger.Info("Starting NATS service")
	return a.srv.Start(ctx)
}

func (a *NATSApp) WaitForStartup() error {
	return a.srv.WaitForStartup()
}
