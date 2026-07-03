package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func publish(ctx context.Context, js jetstream.JetStream, logger *slog.Logger, subject string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON marshal: %w", err)
	}

	for {
		_, err = js.Publish(ctx, subject, b)
		if err == nil {
			return nil
		}
		if !isStreamNotReady(err) {
			return fmt.Errorf("publishing to %s: %w", subject, err)
		}
		logger.Debug("INGEST stream not ready, retrying publish", "subject", subject)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// isStreamNotReady reports whether the publish error is a transient condition
// caused by the INGEST stream not existing yet (e.g. domain service still
// starting up). Both ErrNoResponders and a 404 JetStream APIError indicate
// this state.
func isStreamNotReady(err error) bool {
	if errors.Is(err, gonats.ErrNoResponders) || errors.Is(err, jetstream.ErrNoStreamResponse) {
		return true
	}
	var apiErr *jetstream.APIError
	return errors.As(err, &apiErr) && apiErr.Code == 404
}
