package nats_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

func TestIngestGitRepo(t *testing.T) {
	is := is.New(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	natsURL := dalinats.NATSConnectionString("localhost", 4444)
	ns := dalinats.NewServer()
	ns.Port = 4444
	ns.DataDir = t.TempDir()
	go func() {
		err := ns.Start(t.Context())
		is.NoErr(err)
	}()
	err := ns.WaitForStartup()
	is.NoErr(err)

	nc, err := nats.Connect(natsURL)
	is.NoErr(err)
	js, err := jetstream.New(nc)
	is.NoErr(err)

	svc := domain.NewDomainService(repo.NewMemory(), l)
	sut := dalinats.NewPort(l, svc)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	go func() {
		err := sut.Run(ctx, js)
		is.NoErr(err)
	}()

	for i := range 10 {
		data := []byte("payload")
		t.Logf("Publishing payload: %d", i)
		_, err := js.Publish(ctx, dalinats.SubjectRepo, data)
		is.NoErr(err)
		time.Sleep(time.Millisecond)
	}
}
