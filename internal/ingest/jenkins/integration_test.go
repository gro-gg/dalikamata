//go:build integration

package jenkins_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/app"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	jenkinsfake "codeberg.org/aeforged/dalikamata/internal/ingest/jenkins/fakeserver"
	"codeberg.org/aeforged/dalikamata/internal/testhelper"
	"codeberg.org/aeforged/dalikamata/pkg/model"
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
