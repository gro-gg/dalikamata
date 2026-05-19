package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const (
	MaxDeliver = 5

	StreamIngest       = "ingest.>"
	StreamIngestName   = "INGEST"
	SubjectCommit      = "ingest.git.commit"
	SubjectPullRequest = "ingest.git.pullrequest"
	SubjectRepo        = "ingest.git.repo"

	SubjectPipelineJob   = "ingest.pipeline.job"
	SubjectPipelineBuild = "ingest.pipeline.build"
	SubjectPipelineStage = "ingest.pipeline.stage"

	DefaultHost = "0.0.0.0"
	DefaultPort = 4222

	LogReceivedMessage  = "received message"
	LogHandlerSettingUp = "Repo Handler Setting Up"
)

type NATSPort struct {
	logger  *slog.Logger
	handler domain.GitEventHandler
}

func NATSConnectionString(natsHost string, natsPort int) string {
	return fmt.Sprintf("nats://%s:%d", natsHost, natsPort)
}

func NewPort(logger *slog.Logger, handler domain.GitEventHandler) *NATSPort {
	return &NATSPort{
		logger:  logger.With("type", "port", "component", "ingest_git", "connection", "nats"),
		handler: handler,
	}
}

func (s *NATSPort) Run(ctx context.Context, js jetstream.JetStream) error {
	s.logger.Info("Starting NATS Service")

	ingestStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamIngestName,
		Subjects: []string{StreamIngest},
	})
	if err != nil {
		return fmt.Errorf("creating stream %s: %w", StreamIngestName, err)
	}

	gitRepoConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-repo",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectRepo,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-git-repo consumer: %w", err)
	}

	gitCommitConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-commit",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectCommit,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-git-commit consumer: %w", err)
	}

	gitPRConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-pullrequest",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectPullRequest,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-git-pullrequest consumer: %w", err)
	}

	gitRepoConsumeCtx, err := gitRepoConsumer.Consume(s.gitRepoHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectRepo, err)
	}

	gitCommitConsumeCtx, err := gitCommitConsumer.Consume(s.gitCommitHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectCommit, err)
	}

	gitPRConsumeCtx, err := gitPRConsumer.Consume(s.gitPullRequestHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectPullRequest, err)
	}

	<-ctx.Done()

	gitRepoConsumeCtx.Drain()
	gitCommitConsumeCtx.Drain()
	gitPRConsumeCtx.Drain()
	s.logger.Info("NATS Service Shut Down")

	return nil
}

func (s *NATSPort) gitRepoHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectRepo)
	l.Info(LogHandlerSettingUp)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var repo model.Repo
		if err := json.Unmarshal(msg.Data(), &repo); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.handler.HandleRepo(ctx, repo); err != nil {
			l.Error("handling repo", "repo_id", repo.RepoID, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) gitCommitHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectCommit)
	l.Info("Commit Handler Setting Up")

	return func(msg jetstream.Msg) {
		var commit model.Commit
		if err := json.Unmarshal(msg.Data(), &commit); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.handler.HandleCommit(ctx, commit); err != nil {
			l.Error("handling commit", "sha", commit.SHA, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) gitPullRequestHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPullRequest)
	l.Info("Pull Request Handler Setting Up")

	return func(msg jetstream.Msg) {
		var pr model.PullRequest
		if err := json.Unmarshal(msg.Data(), &pr); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.handler.HandlePullRequest(ctx, pr); err != nil {
			l.Error("handling pull request", "id", pr.ID, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}
