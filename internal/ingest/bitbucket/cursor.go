package bitbucket

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// Cursors tracks the per-repo commit watermark used to resume incremental
// pulls. Save persists the newest known commit SHA; Load hydrates all cursors
// on startup; Clear removes a cursor so the next crawl does a full refetch.
type Cursors interface {
	Load(ctx context.Context) (map[string]string, error)
	Save(ctx context.Context, repoID, sha string) error
	Clear(ctx context.Context, repoID string) error
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

func (c *jetstreamCursors) Load(ctx context.Context) (map[string]string, error) {
	keys, err := c.kv.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("listing cursor keys: %w", err)
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		entry, err := c.kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("loading cursor for %s: %w", key, err)
		}
		result[key] = string(entry.Value())
	}
	return result, nil
}

func (c *jetstreamCursors) Save(ctx context.Context, repoID, sha string) error {
	if _, err := c.kv.Put(ctx, repoID, []byte(sha)); err != nil {
		return fmt.Errorf("saving cursor for %s: %w", repoID, err)
	}
	return nil
}

func (c *jetstreamCursors) Clear(ctx context.Context, repoID string) error {
	err := c.kv.Delete(ctx, repoID)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("clearing cursor for %s: %w", repoID, err)
	}
	return nil
}
