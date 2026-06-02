//go:build integration

package jenkins_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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

	// 4. Assert NATS message counts
	// fakeserver fixture: 5 jobs (build-backend, test-backend, deploy-backend, build-frontend, deploy-frontend)
	jobs := testhelper.CollectMessages[model.Workflow](t, js, dalinats.SubjectCicdWorkflow, 5, 20*time.Second)
	is.Equal(len(jobs), 5)

	// 5 jobs × 10 builds each = 50 builds
	builds := testhelper.CollectMessages[model.WorkflowRun](t, js, dalinats.SubjectCicdWorkflowRun, 50, 20*time.Second)
	is.Equal(len(builds), 50)

	// build-backend(4) + test-backend(3) + deploy-backend(4) + build-frontend(4) + deploy-frontend(4) = 19 stages/cycle × 10 builds = 190
	stages := testhelper.CollectMessages[model.WorkflowTask](t, js, dalinats.SubjectCicdWorkflowTask, 190, 20*time.Second)
	is.Equal(len(stages), 190)
}
