package nats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Connect dials the NATS server at url, retrying every second until the
// connection succeeds or ctx is cancelled. Returns a ready JetStream instance
// and a close function for the underlying connection.
func Connect(ctx context.Context, url string, logger *slog.Logger) (jetstream.JetStream, func(), error) {
	for {
		nc, err := nats.Connect(url)
		if err == nil {
			js, err := jetstream.New(nc)
			if err != nil {
				nc.Close()
				return nil, nil, fmt.Errorf("creating JetStream: %w", err)
			}
			return js, nc.Close, nil
		}
		logger.Error("connecting to NATS", "error", err)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
