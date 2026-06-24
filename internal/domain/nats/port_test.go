package nats_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	testis "github.com/matryer/is"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
)

func collectTeams(t *testing.T, r *repo.MemoryRepository) []model.Team {
	t.Helper()
	var out []model.Team
	if err := r.QueryTeams(context.Background(), query.Query{Entity: query.EntityTeam}, func(tm model.Team) error {
		out = append(out, tm)
		return nil
	}); err != nil {
		t.Fatalf("QueryTeams: %v", err)
	}
	return out
}

func collectComponents(t *testing.T, r *repo.MemoryRepository) []model.Component {
	t.Helper()
	var out []model.Component
	if err := r.QueryComponents(context.Background(), query.Query{Entity: query.EntityComponent}, func(c model.Component) error {
		out = append(out, c)
		return nil
	}); err != nil {
		t.Fatalf("QueryComponents: %v", err)
	}
	return out
}

func startNATS(t *testing.T, port int) (string, jetstream.JetStream) {
	t.Helper()
	is := testis.New(t)
	natsURL := internalnats.NATSConnectionString("localhost", port)
	ns := internalnats.NewServer()
	ns.Port = port
	ns.DataDir = t.TempDir()
	go func() {
		err := ns.Start(t.Context())
		is.NoErr(err)
	}()
	is.NoErr(ns.WaitForStartup())
	nc, err := nats.Connect(natsURL)
	is.NoErr(err)
	js, err := jetstream.New(nc)
	is.NoErr(err)
	return natsURL, js
}

func TestIngestGitRepo(t *testing.T) {
	is := testis.New(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, js := startNATS(t, 4444)

	memory := repo.NewMemory()
	svc := domain.NewDomainService(memory, memory, l)
	sut, err := dalinats.NewPort(l, dalinats.WithGitEventHandler(svc), dalinats.WithCicdEventHandler(svc))
	is.NoErr(err)

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

func TestIngestPlatformTeamAndComponent(t *testing.T) {
	is := testis.New(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, js := startNATS(t, 4445)

	memory := repo.NewMemory()
	svc := domain.NewDomainService(memory, memory, l)
	sut, err := dalinats.NewPort(l,
		dalinats.WithGitEventHandler(svc),
		dalinats.WithCicdEventHandler(svc),
		dalinats.WithPlatformEventHandler(svc),
	)
	is.NoErr(err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	go func() {
		err := sut.Run(ctx, js)
		is.NoErr(err)
	}()

	// team event
	teamJSON := `{"name":"payments"}`
	_, err = js.Publish(ctx, dalinats.SubjectPlatformTeam, []byte(teamJSON))
	is.NoErr(err)

	// component event
	compJSON := `{"name":"payment-service","team_name":"payments","repos":[],"workflows":[]}`
	_, err = js.Publish(ctx, dalinats.SubjectPlatformComponent, []byte(compJSON))
	is.NoErr(err)

	// allow consumers to process
	time.Sleep(100 * time.Millisecond)

	teams := collectTeams(t, memory)
	// QueryTeams always includes the synthetic "unknown" team.
	is.Equal(len(teams), 2)
	teamNames := make(map[string]bool, len(teams))
	for _, tm := range teams {
		teamNames[tm.Name] = true
	}
	is.True(teamNames["payments"])
	is.True(teamNames["unknown"])

	comps := collectComponents(t, memory)
	is.Equal(len(comps), 1)
	is.Equal(comps[0].Name, "payment-service")
}
