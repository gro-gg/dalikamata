package app

import (
	"context"
	"log/slog"

	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
)

type NATSApp struct {
	Host    string
	Port    int
	DataDir string
	logger  *slog.Logger
	srv     *internalnats.Server
}

func NewNATSApp(logger *slog.Logger) *NATSApp {
	return &NATSApp{
		Host:    "0.0.0.0",
		Port:    4222,
		DataDir: "./data/nats",
		logger:  logger.With("service", "nats"),
		srv:     internalnats.NewServer(),
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
