package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const (
	MaxDeliver = 5

	StreamIngest       = "ingest.>"
	StreamIngestName   = "INGEST"
	SubjectCommit      = "ingest.git.commit"
	SubjectPullRequest = "ingest.git.pullrequest"
	SubjectRepo        = "ingest.git.repo"

	DefaultHost = "0.0.0.0"
	DefaultPort = 4222
)

type NATSPort struct {
	logger             *slog.Logger
	natsAddr           string
	natsPort           int
	natsStartupTimeout time.Duration
	jetstreamDir       string
}

func NATSConnectionString(natsHost string, natsPort int) string {
	return fmt.Sprintf("nats://%s:%d", natsHost, natsPort)
}

func NewPort(logger *slog.Logger) *NATSPort {
	s := &NATSPort{
		logger: logger,
	}

	return s
}

func (s *NATSPort) Run(ctx context.Context, js jetstream.JetStream) (func(), error) {
	s.logger.Info("Starting NATS Service")

	ingestStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamIngestName,
		Subjects: []string{StreamIngest},
	})
	if err != nil {
		return nil, fmt.Errorf("creating stream %s: %w", StreamIngestName, err)
	}

	gitRepoConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-repo",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectRepo,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return nil, fmt.Errorf("creating ingest-git-repo consumer: %w", err)
	}

	gitRepoConsumeCtx, err := gitRepoConsumer.Consume(s.gitRepoHandler())
	if err != nil {
		return nil, fmt.Errorf("starting %s consumer: %w", SubjectRepo, err)
	}

	shutdownFunc := func() {
		gitRepoConsumeCtx.Drain()
		s.logger.Info("NATS Service Shut Down")
	}

	return shutdownFunc, nil
}

func (s *NATSPort) gitRepoHandler() func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectRepo)
	l.Info("Repo Handler Settiing Up")

	return func(msg jetstream.Msg) {
		var repo model.Repo
		err := json.Unmarshal(msg.Data(), &repo)
		if err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			msg.Nak()
			return
		}
		l.Info("received repo", "payload", repo)
		msg.Ack()
	}
}
