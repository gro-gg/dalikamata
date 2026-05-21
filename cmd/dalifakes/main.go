package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd.SetContext(app.AddWaitGroup(ctx))

	err := rootCmd.Execute()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}
