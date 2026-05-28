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
// connection succeeds or ctx is cancelled. Returns the underlying connection
// (for core NATS operations such as request-reply), a ready JetStream
// instance, and a close function for the connection.
func Connect(ctx context.Context, url string, logger *slog.Logger) (*nats.Conn, jetstream.JetStream, func(), error) {
	for {
		nc, err := nats.Connect(url)
		if err == nil {
			js, err := jetstream.New(nc)
			if err != nil {
				nc.Close()
				return nil, nil, nil, fmt.Errorf("creating JetStream: %w", err)
			}
			return nc, js, nc.Close, nil
		}
		logger.Error("connecting to NATS", "error", err)
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
