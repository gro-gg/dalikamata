package jenkins

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go/jetstream"
)

// Cursors tracks the per-job build-number watermark used to resume incremental
// pulls. Save persists the newest completed build number; Load hydrates all
// cursors on startup; Clear removes a cursor so the next crawl does a full refetch.
type Cursors interface {
	Load(ctx context.Context) (map[string]int, error)
	Save(ctx context.Context, jobPath string, number int) error
	Clear(ctx context.Context, jobPath string) error
}

// jetstreamCursors is the production Cursors backed by a JetStream KV bucket.
type jetstreamCursors struct {
	kv jetstream.KeyValue
}

// NewJetStreamCursors creates or updates a JetStream KV bucket with the given
// name and returns a Cursors backed by it. Safe to call on restart — the bucket
// is retained and prior cursor values are preserved.
func NewJetStreamCursors(ctx context.Context, js jetstream.JetStream, bucketName string) (Cursors, error) {
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		return nil, fmt.Errorf("creating KV bucket %s: %w", bucketName, err)
	}
	return &jetstreamCursors{kv: kv}, nil
}

func (c *jetstreamCursors) Load(ctx context.Context) (map[string]int, error) {
	keys, err := c.kv.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return map[string]int{}, nil
		}
		return nil, fmt.Errorf("listing cursor keys: %w", err)
	}
	result := make(map[string]int, len(keys))
	for _, key := range keys {
		entry, err := c.kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("loading cursor for %s: %w", key, err)
		}
		n, err := strconv.Atoi(string(entry.Value()))
		if err != nil {
			return nil, fmt.Errorf("parsing cursor for %s: %w", key, err)
		}
		result[key] = n
	}
	return result, nil
}

func (c *jetstreamCursors) Save(ctx context.Context, jobPath string, number int) error {
	if _, err := c.kv.Put(ctx, jobPath, []byte(strconv.Itoa(number))); err != nil {
		return fmt.Errorf("saving cursor for %s: %w", jobPath, err)
	}
	return nil
}

func (c *jetstreamCursors) Clear(ctx context.Context, jobPath string) error {
	err := c.kv.Delete(ctx, jobPath)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("clearing cursor for %s: %w", jobPath, err)
	}
	return nil
}
