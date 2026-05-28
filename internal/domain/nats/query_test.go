package nats_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	testis "github.com/matryer/is"
	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
	"codeberg.org/aeforged/dalikamata/internal/testhelper"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// startQueryStack spins up an embedded NATS server, connects, wires up the
// full domain service, starts both ports, and seeds data.
// Returns a connected QueryClient and a cancel func for cleanup.
func startQueryStack(t *testing.T) (*dalinats.QueryClient, func()) {
	t.Helper()
	is := testis.New(t)

	natsURL, _ := testhelper.StartNATS(t)

	// One connection shared by both ports (JetStream is built on core NATS).
	nc, err := gonats.Connect(natsURL)
	is.NoErr(err)

	js, err := jetstream.New(nc)
	is.NoErr(err)

	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	memory := repo.NewMemory()
	svc := domain.NewDomainService(memory, memory, l)

	ingestPort := dalinats.NewPort(l, dalinats.WithGitEventHandler(svc), dalinats.WithCicdEventHandler(svc))
	queryPort := dalinats.NewQueryPort(l, svc)

	ctx, cancel := context.WithCancel(context.Background())

	go func() { _ = ingestPort.Run(ctx, js) }()
	go func() { _ = queryPort.Run(ctx, nc) }()

	// Seed test data directly via the service.
	seedCtx := context.Background()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	is.NoErr(svc.HandleCommit(seedCtx, model.Commit{SHA: "aaa", RepoID: "X/repo", Author: "alice", Timestamp: t0}))
	is.NoErr(svc.HandleCommit(seedCtx, model.Commit{SHA: "bbb", RepoID: "Y/repo", Author: "bob", Timestamp: t0.Add(24 * time.Hour)}))
	is.NoErr(svc.HandleCommit(seedCtx, model.Commit{SHA: "ccc", RepoID: "X/repo", Author: "carol", Timestamp: t0.Add(48 * time.Hour)}))

	is.NoErr(svc.HandlePullRequest(seedCtx, model.PullRequest{ID: "X/repo/1", RepoID: "X/repo", State: model.PullRequestStateMerged}))
	is.NoErr(svc.HandlePullRequest(seedCtx, model.PullRequest{ID: "X/repo/2", RepoID: "X/repo", State: model.PullRequestStateOpen}))

	// Allow the port goroutines time to subscribe and flush to the server.
	time.Sleep(20 * time.Millisecond)

	client := dalinats.NewQueryClient(nc, l, dalinats.WithQueryTimeout(5*time.Second))

	return client, func() {
		cancel()
		nc.Close()
	}
}

// TestQueryCommits_TermFilter filters commits by repo_id.
func TestQueryCommits_TermFilter(t *testing.T) {
	is := testis.New(t)
	client, cleanup := startQueryStack(t)
	defer cleanup()

	q := query.Query{
		Entity: query.EntityCommit,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.CommitRepoID,
			Value: ptrQ(query.StringValue("X/repo")),
		},
	}

	got, err := client.QueryCommitsAll(context.Background(), q)
	is.NoErr(err)
	is.Equal(len(got), 2)
	for _, c := range got {
		is.Equal(c.RepoID, "X/repo")
	}
}

// TestQueryCommits_RangeFilter filters commits by timestamp range.
func TestQueryCommits_RangeFilter(t *testing.T) {
	is := testis.New(t)
	client, cleanup := startQueryStack(t)
	defer cleanup()

	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t0.Add(30 * time.Hour)

	q := query.Query{
		Entity: query.EntityCommit,
		Filter: &query.Filter{
			Op:    query.OpRange,
			Field: query.CommitTimestamp,
			Range: &query.Range{
				GTE: ptrQ(query.TimeValue(t0)),
				LTE: ptrQ(query.TimeValue(t2)),
			},
		},
	}

	got, err := client.QueryCommitsAll(context.Background(), q)
	is.NoErr(err)
	is.Equal(len(got), 2) // aaa at t0, bbb at t0+24h — ccc at t0+48h excluded
}

// TestQueryCommits_EmptyFilter returns all commits.
func TestQueryCommits_EmptyFilter(t *testing.T) {
	is := testis.New(t)
	client, cleanup := startQueryStack(t)
	defer cleanup()

	got, err := client.QueryCommitsAll(context.Background(), query.Query{Entity: query.EntityCommit})
	is.NoErr(err)
	is.Equal(len(got), 3)
}

// TestQueryCommits_SortAndPaginate sorts by timestamp desc and takes page 1.
func TestQueryCommits_SortAndPaginate(t *testing.T) {
	is := testis.New(t)
	client, cleanup := startQueryStack(t)
	defer cleanup()

	q := query.Query{
		Entity: query.EntityCommit,
		Sort:   []query.SortField{{Field: query.CommitTimestamp, Order: query.SortDesc}},
		From:   0,
		Size:   2,
	}

	got, err := client.QueryCommitsAll(context.Background(), q)
	is.NoErr(err)
	is.Equal(len(got), 2)
	// ccc has the latest timestamp — must come first.
	is.Equal(got[0].SHA, "ccc")
}

// TestQueryPullRequests_TermsFilter filters PRs by a set of states.
func TestQueryPullRequests_TermsFilter(t *testing.T) {
	is := testis.New(t)
	client, cleanup := startQueryStack(t)
	defer cleanup()

	q := query.Query{
		Entity: query.EntityPullRequest,
		Filter: &query.Filter{
			Op:    query.OpTerms,
			Field: query.PRState,
			Values: []query.Value{
				query.StringValue(model.PullRequestStateMerged),
			},
		},
	}

	got, err := client.QueryPullRequestsAll(context.Background(), q)
	is.NoErr(err)
	is.Equal(len(got), 1)
	is.Equal(got[0].State, model.PullRequestStateMerged)
}

// TestQueryCommits_ZeroMatch confirms a clean done when nothing matches.
func TestQueryCommits_ZeroMatch(t *testing.T) {
	is := testis.New(t)
	client, cleanup := startQueryStack(t)
	defer cleanup()

	q := query.Query{
		Entity: query.EntityCommit,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.CommitRepoID,
			Value: ptrQ(query.StringValue("DOES/NOT/EXIST")),
		},
	}

	out, errs := client.QueryCommits(context.Background(), q)
	var count int
	for range out {
		count++
	}
	is.NoErr(<-errs)
	is.Equal(count, 0)
}

// TestQueryCommits_MalformedRequest confirms the error path when the server
// cannot decode the request.
func TestQueryCommits_MalformedRequest(t *testing.T) {
	is := testis.New(t)

	natsURL, _ := testhelper.StartNATS(t)
	nc, err := gonats.Connect(natsURL)
	is.NoErr(err)
	defer nc.Close()

	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	memory := repo.NewMemory()
	svc := domain.NewDomainService(memory, memory, l)
	qp := dalinats.NewQueryPort(l, svc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = qp.Run(ctx, nc) }()

	// Give the port a moment to subscribe.
	time.Sleep(10 * time.Millisecond)

	inbox := nc.NewInbox()
	sub, err := nc.SubscribeSync(inbox)
	is.NoErr(err)
	defer sub.Unsubscribe() //nolint:errcheck

	is.NoErr(nc.PublishRequest(dalinats.SubjectQueryCommit, inbox, []byte("not json {")))

	msg, err := sub.NextMsgWithContext(context.Background())
	is.NoErr(err)
	is.Equal(msg.Header.Get(dalinats.HeaderQueryStatus), dalinats.StatusError)

	var body struct{ Error string `json:"error"` }
	is.NoErr(json.Unmarshal(msg.Data, &body))
	is.True(body.Error != "")
}

// TestQueryCommits_ServerStartedAfterNATS uses a second plain *nats.Conn to
// test that the query port connects without requiring the embedded NATS server
// to be pre-configured with a stream — the query path does not use JetStream.
func TestQuery_CoreNATSOnly(t *testing.T) {
	is := testis.New(t)

	natsPort := testhelper.FreePort(t)
	natsURL := internalnats.NATSConnectionString("127.0.0.1", natsPort)

	ns := internalnats.NewServer()
	ns.Port = natsPort
	ns.DataDir = t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = ns.Start(ctx) }()
	is.NoErr(ns.WaitForStartup())

	nc, err := gonats.Connect(natsURL)
	is.NoErr(err)
	defer nc.Close()

	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	memory := repo.NewMemory()
	svc := domain.NewDomainService(memory, memory, l)

	// Seed without needing JetStream.
	is.NoErr(memory.AddRepo(ctx, model.Repo{RepoID: "P/r", Name: "myrepo"}))

	qp := dalinats.NewQueryPort(l, svc)
	go func() { _ = qp.Run(ctx, nc) }()

	// Give subscriptions time to register.
	time.Sleep(10 * time.Millisecond)

	client := dalinats.NewQueryClient(nc, l, dalinats.WithQueryTimeout(5*time.Second))
	got, err := client.QueryReposAll(ctx, query.Query{Entity: query.EntityRepo})
	is.NoErr(err)
	is.Equal(len(got), 1)
	is.Equal(got[0].RepoID, "P/r")
}

func ptrQ[T any](v T) *T { return &v }
