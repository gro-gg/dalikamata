//go:build integration

package bitbucket_test

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
	"codeberg.org/aeforged/dalikamata/internal/ingest/bitbucket/fakeserver"
	"codeberg.org/aeforged/dalikamata/internal/testhelper"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

func TestIngestBitbucketIntegration(t *testing.T) {
	t.Parallel()

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
