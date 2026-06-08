//go:build integration

package bitbucket_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/app"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/httpclient"
	"codeberg.org/aeforged/dalikamata/internal/ingest/bitbucket"
	"codeberg.org/aeforged/dalikamata/internal/ingest/bitbucket/fakeserver"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
	"codeberg.org/aeforged/dalikamata/internal/testhelper"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

func TestIngestBitbucketIntegration(t *testing.T) {
	is := is.New(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Embedded NATS + INGEST stream
	natsURL, natsPort := testhelper.StartNATS(t)
	js := testhelper.NewJetStream(t, natsURL)
	testhelper.CreateIngestStream(t, js)

	// 2. In-process fake Bitbucket server
	bbPort := testhelper.FreePort(t)
	bbSrv := fakeserver.New(fmt.Sprintf("127.0.0.1:%d", bbPort), l.With("service", "fake-bitbucket"))
	bbCtx, bbCancel := context.WithCancel(t.Context())
	t.Cleanup(bbCancel)
	go func() {
		if err := bbSrv.Start(bbCtx); err != nil {
			t.Logf("fake bitbucket stopped: %v", err)
		}
	}()
	testhelper.WaitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d/rest/api/1.0/projects/PROJ/repos", bbPort))

	// 3. Bitbucket ingest app
	ingestApp := app.NewIngestBitbucketApp(l)
	ingestApp.NATSHost = "127.0.0.1"
	ingestApp.NATSPort = natsPort
	ingestApp.BitbucketURL = fmt.Sprintf("http://127.0.0.1:%d", bbPort)
	ingestApp.BitbucketToken = "test-token"
	ingestApp.Projects = []string{"PROJ", "INFRA"}

	ingestCtx, ingestCancel := context.WithCancel(t.Context())
	t.Cleanup(ingestCancel)
	go func() {
		if err := ingestApp.Run(ingestCtx); err != nil {
			t.Logf("ingest app stopped: %v", err)
		}
	}()

	// 4. Assert NATS message counts
	// fakeserver fixture: PROJ(3 repos) + INFRA(2 repos) = 5
	repos := testhelper.CollectMessages[model.Repo](t, js, dalinats.SubjectRepo, 5, 10*time.Second)
	is.Equal(len(repos), 5)

	// backend-api(5) + frontend-app(3) + shared-lib(2) + k8s-configs(2) + terraform-modules(3) = 15
	commits := testhelper.CollectMessages[model.Commit](t, js, dalinats.SubjectCommit, 15, 10*time.Second)
	is.Equal(len(commits), 15)

	// backend-api(3) + frontend-app(2) + shared-lib(1) + k8s-configs(1) + terraform-modules(2) = 9
	prs := testhelper.CollectMessages[model.PullRequest](t, js, dalinats.SubjectPullRequest, 9, 10*time.Second)
	is.Equal(len(prs), 9)
}

// TestIngestBitbucketIncremental verifies the per-repo commit cursor:
//  1. First crawl publishes all fixture commits (15).
//  2. After AddCommit, a second crawl publishes exactly 1 new commit.
//  3. A fresh crawler that reloads cursors from the KV store publishes 0
//     new commits — proving the cursor survived the simulated restart.
func TestIngestBitbucketIncremental(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Embedded NATS + INGEST stream
	natsURL, _ := testhelper.StartNATS(t)
	js := testhelper.NewJetStream(t, natsURL)
	testhelper.CreateIngestStream(t, js)

	// 2. In-process fake Bitbucket server
	bbPort := testhelper.FreePort(t)
	bbSrv := fakeserver.New(fmt.Sprintf("127.0.0.1:%d", bbPort), l.With("service", "fake-bitbucket"))
	bbCtx, bbCancel := context.WithCancel(ctx)
	t.Cleanup(bbCancel)
	go func() {
		if err := bbSrv.Start(bbCtx); err != nil {
			t.Logf("fake bitbucket stopped: %v", err)
		}
	}()
	testhelper.WaitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d/rest/api/1.0/projects/PROJ/repos", bbPort))

	// 3. Wire components manually for precise crawl control.
	publisher, publisherCloser, err := dalinats.NewGitPublisher(ctx, natsURL, l)
	is.NoErr(err)
	t.Cleanup(publisherCloser)

	_, jsForCursors, jsCloser, err := internalnats.Connect(ctx, natsURL, l)
	is.NoErr(err)
	t.Cleanup(jsCloser)

	cursors, err := bitbucket.NewJetStreamCursors(ctx, jsForCursors, "test-bb-cursors")
	is.NoErr(err)

	httpCl, err := httpclient.NewHTTPClient("")
	is.NoErr(err)
	bbURL := fmt.Sprintf("http://127.0.0.1:%d", bbPort)
	client := bitbucket.NewClient(bbURL, "test-token", httpCl, l)
	projects := []string{"PROJ", "INFRA"}
	crawler := bitbucket.NewCrawler(client, publisher, cursors, projects, l)

	// 4. First crawl: all 15 fixture commits must be published.
	is.NoErr(crawler.Crawl(ctx))
	_ = testhelper.CollectMessages[model.Commit](t, js, dalinats.SubjectCommit, 15, 10*time.Second)

	// 5. Inject a new commit and run a second crawl.
	bbSrv.AddCommit("backend-api", fakeserver.NewCommit("new-commit-sha-01", "feat: incremental test"))

	ingestStream, err := js.Stream(ctx, dalinats.StreamIngestName)
	is.NoErr(err)
	streamInfo, err := ingestStream.Info(ctx)
	is.NoErr(err)
	seqBeforeCrawl2 := streamInfo.State.LastSeq

	is.NoErr(crawler.Crawl(ctx))

	// Only the 1 new commit should have been published (not the 15 old ones).
	newCommits := collectCommitsSince(t, is, ingestStream, seqBeforeCrawl2+1)
	is.Equal(len(newCommits), 1)
	is.Equal(newCommits[0].SHA, "new-commit-sha-01")

	// 6. Simulate restart: build a fresh crawler backed by the same KV bucket.
	publisher2, publisherCloser2, err := dalinats.NewGitPublisher(ctx, natsURL, l)
	is.NoErr(err)
	t.Cleanup(publisherCloser2)

	cursors2, err := bitbucket.NewJetStreamCursors(ctx, jsForCursors, "test-bb-cursors")
	is.NoErr(err)

	crawler2 := bitbucket.NewCrawler(client, publisher2, cursors2, projects, l)

	streamInfo2, err := ingestStream.Info(ctx)
	is.NoErr(err)
	seqBeforeCrawl3 := streamInfo2.State.LastSeq

	is.NoErr(crawler2.Crawl(ctx))

	// Cursor survived the restart: no new commits re-published.
	restartCommits := collectCommitsSince(t, is, ingestStream, seqBeforeCrawl3+1)
	is.Equal(len(restartCommits), 0)
}

// collectCommitsSince returns all commit messages on the INGEST stream at or
// after startSeq. Uses FetchNoWait so it only returns messages already stored;
// the caller must ensure the crawl has completed before calling this.
func collectCommitsSince(t *testing.T, is *is.I, stream jetstream.Stream, startSeq uint64) []model.Commit {
	t.Helper()
	ctx := context.Background()

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: dalinats.SubjectCommit,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:   startSeq,
	})
	is.NoErr(err)

	batch, err := consumer.FetchNoWait(1000)
	is.NoErr(err)

	var commits []model.Commit
	for msg := range batch.Messages() {
		var c model.Commit
		is.NoErr(json.Unmarshal(msg.Data(), &c))
		commits = append(commits, c)
		_ = msg.Ack()
	}
	return commits
}
