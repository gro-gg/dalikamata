package testhelper

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
)

// FreePort returns an available TCP port on 127.0.0.1.
func FreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// WaitHTTP polls url until it returns HTTP 200 or a 10-second deadline elapses.
func WaitHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("service at %s did not become ready within 10s", url)
}

// StartNATS starts an embedded NATS JetStream server on a free port.
// Returns the NATS URL and port number. The server stops when t finishes.
func StartNATS(t *testing.T) (natsURL string, port int) {
	t.Helper()
	port = FreePort(t)
	ns := internalnats.NewServer()
	ns.Port = port
	ns.DataDir = t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		if err := ns.Start(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("NATS server error: %v", err)
		}
	}()

	if err := ns.WaitForStartup(); err != nil {
		t.Fatalf("NATS startup failed: %v", err)
	}

	return internalnats.NATSConnectionString("127.0.0.1", port), port
}

// NewJetStream connects to natsURL and returns a ready JetStream instance.
// The connection is closed when t finishes.
func NewJetStream(t *testing.T, natsURL string) jetstream.JetStream {
	t.Helper()
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connecting to NATS: %v", err)
	}
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("creating JetStream: %v", err)
	}
	return js
}

// CreateIngestStream creates the INGEST JetStream stream that covers all
// ingest.> subjects. Must be called before the ingest app publishes.
func CreateIngestStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	_, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     dalinats.StreamIngestName,
		Subjects: []string{dalinats.StreamIngest},
	})
	if err != nil {
		t.Fatalf("creating INGEST stream: %v", err)
	}
}

// CollectMessages creates a consumer on the INGEST stream filtered to subject
// and collects exactly n messages, JSON-decoding each as T. Fails t if timeout
// elapses before n messages arrive. Uses DeliverAllPolicy so messages published
// before the consumer is created are also delivered.
func CollectMessages[T any](t *testing.T, js jetstream.JetStream, subject string, n int, timeout time.Duration) []T {
	t.Helper()

	ctx := context.Background()
	stream, err := js.Stream(ctx, dalinats.StreamIngestName)
	if err != nil {
		t.Fatalf("getting INGEST stream: %v", err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatalf("creating consumer for %s: %v", subject, err)
	}

	ch := make(chan T, n)
	cctx, err := consumer.Consume(func(msg jetstream.Msg) {
		var item T
		if err := json.Unmarshal(msg.Data(), &item); err != nil {
			t.Errorf("unmarshal %s message: %v", subject, err)
		} else {
			ch <- item
		}
		_ = msg.Ack()
	})
	if err != nil {
		t.Fatalf("starting consumer for %s: %v", subject, err)
	}
	t.Cleanup(cctx.Stop)

	results := make([]T, 0, n)
	deadline := time.After(timeout)
	for len(results) < n {
		select {
		case item := <-ch:
			results = append(results, item)
		case <-deadline:
			t.Fatalf("CollectMessages(%s): timed out after %s; got %d of %d messages", subject, timeout, len(results), n)
		}
	}
	return results
}
