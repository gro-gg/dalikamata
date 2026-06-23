//go:build integration

package jenkins_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/matryer/is"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/app"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/httpclient"
	"codeberg.org/aeforged/dalikamata/internal/ingest/jenkins"
	jenkinsfake "codeberg.org/aeforged/dalikamata/internal/ingest/jenkins/fakeserver"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
	"codeberg.org/aeforged/dalikamata/internal/testhelper"
)

func TestIngestJenkinsIntegration(t *testing.T) {
	is := is.New(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Embedded NATS + INGEST stream
	natsURL, natsPort := testhelper.StartNATS(t)
	js := testhelper.NewJetStream(t, natsURL)
	testhelper.CreateIngestStream(t, js)

	// 2. In-process fake Jenkins server
	jkPort := testhelper.FreePort(t)
	jkSrv := jenkinsfake.New(fmt.Sprintf("127.0.0.1:%d", jkPort), l.With("service", "fake-jenkins"))
	jkCtx, jkCancel := context.WithCancel(t.Context())
	t.Cleanup(jkCancel)
	go func() {
		if err := jkSrv.Start(jkCtx); err != nil {
			t.Logf("fake jenkins stopped: %v", err)
		}
	}()
	testhelper.WaitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d/api/json", jkPort))

	// 3. Jenkins ingest app
	ingestApp := app.NewIngestJenkinsApp(l)
	ingestApp.NATSHost = "127.0.0.1"
	ingestApp.NATSPort = natsPort
	ingestApp.JenkinsURL = fmt.Sprintf("http://127.0.0.1:%d", jkPort)
	ingestApp.JenkinsUser = "test"
	ingestApp.JenkinsToken = "test-token"

	ingestCtx, ingestCancel := context.WithCancel(t.Context())
	t.Cleanup(ingestCancel)
	go func() {
		if err := ingestApp.Run(ingestCtx); err != nil {
			t.Logf("ingest app stopped: %v", err)
		}
	}()

	// 4. Assert NATS message counts.
	// fakeserver fixture:
	//   5 plain WorkflowJobs + 1 MultibranchPipeline (shared-lib) with 2 branches
	//   → 6 Workflow messages (shared-lib/main and shared-lib/hotfix deduplicate to one)
	jobs := testhelper.CollectMessages[model.Workflow](t, js, dalinats.SubjectCicdWorkflow, 6, 20*time.Second)
	is.Equal(len(jobs), 6)

	// Every workflow must carry a non-empty RepoID in projectKey/slug form.
	for _, wf := range jobs {
		is.True(wf.RepoID != "") // RepoID must be populated from remote URL
	}
	// Workflow ID must never contain a branch leaf (no "shared-lib/main" etc.)
	for _, wf := range jobs {
		is.True(wf.ID == wf.Name) // Name == ID per current convention
	}
	// Backend, frontend, and shared-lib each resolve to a distinct repo.
	repoIDs := map[string]bool{}
	for _, wf := range jobs {
		repoIDs[wf.RepoID] = true
	}
	is.Equal(len(repoIDs), 3) // ACME/backend, ACME/frontend, ACME/shared-lib

	// 5 plain jobs × 10 builds + 2 branches × 3 builds = 56 workflow runs
	runs := testhelper.CollectMessages[model.WorkflowRun](t, js, dalinats.SubjectCicdWorkflowRun, 56, 20*time.Second)
	is.Equal(len(runs), 56)

	// All shared-lib runs must reference the pipeline ID, not a branch path.
	for _, r := range runs {
		if strings.HasPrefix(r.ID, "shared-lib/") {
			is.Equal(r.WorkflowID, "shared-lib")
		}
	}

	// build-backend(4) + test-backend(3) + deploy-backend(4) + build-frontend(4) + deploy-frontend(4) = 19 stages × 10 builds = 190
	// shared-lib branches return no stages.
	stages := testhelper.CollectMessages[model.WorkflowTask](t, js, dalinats.SubjectCicdWorkflowTask, 190, 20*time.Second)
	is.Equal(len(stages), 190)
}

func TestIngestJenkins_ExplicitBranchStripsName(t *testing.T) {
	// When a single branch of a MultibranchPipeline is given in Jobs, the
	// published Workflow must carry the parent pipeline name, not the branch path.
	is := is.New(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	natsURL, natsPort := testhelper.StartNATS(t)
	js := testhelper.NewJetStream(t, natsURL)
	testhelper.CreateIngestStream(t, js)

	jkPort := testhelper.FreePort(t)
	jkSrv := jenkinsfake.New(fmt.Sprintf("127.0.0.1:%d", jkPort), l.With("service", "fake-jenkins"))
	jkCtx, jkCancel := context.WithCancel(t.Context())
	t.Cleanup(jkCancel)
	go func() {
		if err := jkSrv.Start(jkCtx); err != nil {
			t.Logf("fake jenkins stopped: %v", err)
		}
	}()
	testhelper.WaitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d/api/json", jkPort))

	ingestApp := app.NewIngestJenkinsApp(l)
	ingestApp.NATSHost = "127.0.0.1"
	ingestApp.NATSPort = natsPort
	ingestApp.JenkinsURL = fmt.Sprintf("http://127.0.0.1:%d", jkPort)
	ingestApp.JenkinsUser = "test"
	ingestApp.JenkinsToken = "test-token"
	ingestApp.Jobs = []string{"shared-lib/main"}

	ingestCtx, ingestCancel := context.WithCancel(t.Context())
	t.Cleanup(ingestCancel)
	go func() {
		if err := ingestApp.Run(ingestCtx); err != nil {
			t.Logf("ingest app stopped: %v", err)
		}
	}()

	// One branch configured → one Workflow, three runs (shared-lib/main has 3 builds).
	workflows := testhelper.CollectMessages[model.Workflow](t, js, dalinats.SubjectCicdWorkflow, 1, 20*time.Second)
	is.Equal(len(workflows), 1)
	is.Equal(workflows[0].ID, "shared-lib")
	is.Equal(workflows[0].Name, "shared-lib")

	runs := testhelper.CollectMessages[model.WorkflowRun](t, js, dalinats.SubjectCicdWorkflowRun, 3, 20*time.Second)
	is.Equal(len(runs), 3)
	for _, r := range runs {
		is.Equal(r.WorkflowID, "shared-lib") // must reference parent, not branch path
	}
}

// TestIngestJenkinsIncremental verifies the cursor-based watermark end-to-end:
//  1. First Crawl: all fixture builds published, cursor persisted.
//  2. Second Crawl after AddBuild: exactly 1 new WorkflowRun published.
//  3. Third Crawl with a fresh Crawler reusing the same KV bucket: 0 new runs
//     (proves the cursor survived the simulated restart).
func TestIngestJenkinsIncremental(t *testing.T) {
	is := is.New(t)
	ctx := t.Context()
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	natsURL, _ := testhelper.StartNATS(t)
	js := testhelper.NewJetStream(t, natsURL)
	testhelper.CreateIngestStream(t, js)

	jkPort := testhelper.FreePort(t)
	jkSrv := jenkinsfake.New(fmt.Sprintf("127.0.0.1:%d", jkPort), l.With("service", "fake-jenkins"))
	jkCtx, jkCancel := context.WithCancel(ctx)
	t.Cleanup(jkCancel)
	go func() {
		if err := jkSrv.Start(jkCtx); err != nil {
			t.Logf("fake jenkins stopped: %v", err)
		}
	}()
	testhelper.WaitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d/api/json", jkPort))

	// Wire components manually so we drive Crawl() calls directly.
	publisher, publisherCloser, err := dalinats.NewPipelinePublisher(ctx, natsURL, l)
	is.NoErr(err)
	t.Cleanup(publisherCloser)

	_, jsForCursors, jsCloser, err := internalnats.Connect(ctx, natsURL, l)
	is.NoErr(err)
	t.Cleanup(jsCloser)

	cursors, err := jenkins.NewJetStreamCursors(ctx, jsForCursors, "test-jenkins-cursors")
	is.NoErr(err)

	httpCl, err := httpclient.NewHTTPClient("")
	is.NoErr(err)
	jkURL := fmt.Sprintf("http://127.0.0.1:%d", jkPort)
	client := jenkins.NewClient(jkURL, "test", "test-token", httpCl, l)
	crawler := jenkins.NewCrawler(client, publisher, cursors, nil, l)

	// 1. First crawl: all fixture builds published.
	is.NoErr(crawler.Crawl(ctx))
	// 5 plain jobs × 10 builds + 2 branches × 3 builds = 56 runs.
	_ = testhelper.CollectMessages[model.WorkflowRun](t, js, dalinats.SubjectCicdWorkflowRun, 56, 15*time.Second)

	// 2. Inject a new build and run a second crawl. Assert exactly 1 new run.
	jkSrv.AddBuild("build-backend", jenkinsfake.NewBuild(11, "SUCCESS"))

	ingestStream, err := js.Stream(ctx, dalinats.StreamIngestName)
	is.NoErr(err)
	streamInfo, err := ingestStream.Info(ctx)
	is.NoErr(err)
	seqBeforeCrawl2 := streamInfo.State.LastSeq

	is.NoErr(crawler.Crawl(ctx))

	newRuns := collectRunsSince(t, is, ingestStream, seqBeforeCrawl2+1)
	is.Equal(len(newRuns), 1)
	is.Equal(newRuns[0].Number, 11)

	// 3. Simulate restart: fresh crawler against the same KV bucket → 0 new runs.
	publisher2, publisherCloser2, err := dalinats.NewPipelinePublisher(ctx, natsURL, l)
	is.NoErr(err)
	t.Cleanup(publisherCloser2)

	cursors2, err := jenkins.NewJetStreamCursors(ctx, jsForCursors, "test-jenkins-cursors")
	is.NoErr(err)

	crawler2 := jenkins.NewCrawler(client, publisher2, cursors2, nil, l)

	streamInfo2, err := ingestStream.Info(ctx)
	is.NoErr(err)
	seqBeforeCrawl3 := streamInfo2.State.LastSeq

	is.NoErr(crawler2.Crawl(ctx))

	restartRuns := collectRunsSince(t, is, ingestStream, seqBeforeCrawl3+1)
	is.Equal(len(restartRuns), 0)
}

// collectRunsSince returns all WorkflowRun messages on the INGEST stream at or
// after startSeq. Uses FetchNoWait so it only returns already-stored messages.
func collectRunsSince(t *testing.T, is *is.I, stream jetstream.Stream, startSeq uint64) []model.WorkflowRun {
	t.Helper()
	ctx := context.Background()

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: dalinats.SubjectCicdWorkflowRun,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:   startSeq,
	})
	is.NoErr(err)

	batch, err := consumer.FetchNoWait(1000)
	is.NoErr(err)

	var runs []model.WorkflowRun
	for msg := range batch.Messages() {
		var r model.WorkflowRun
		is.NoErr(json.Unmarshal(msg.Data(), &r))
		runs = append(runs, r)
		_ = msg.Ack()
	}
	return runs
}
