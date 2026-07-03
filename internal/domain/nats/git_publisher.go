package nats

import (
	"context"
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/nats"
)

type GITPublisher struct {
	stream    jetstream.JetStream
	logger    *slog.Logger
	closeConn func()
}

func NewGitPublisher(ctx context.Context, natsURL string, logger *slog.Logger) (domain.GitPublisher, func(), error) {
	_, js, closeConn, err := nats.Connect(ctx, natsURL, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("git publisher connecting to NATS: %w", err)
	}
	logger.Info("Connected to NATS")

	p := &GITPublisher{
		logger:    logger,
		stream:    js,
		closeConn: closeConn,
	}

	return p, p.Close, nil
}

func (p *GITPublisher) Close() {
	p.closeConn()
}

func (p *GITPublisher) PublishRepo(ctx context.Context, repo model.Repo) error {
	p.logger.Debug("publishing repo", "subject", SubjectRepo, "id", repo.RepoID, "name", repo.Name)
	return publish(ctx, p.stream, p.logger, SubjectRepo, repo)
}

func (p *GITPublisher) PublishCommit(ctx context.Context, commit model.Commit) error {
	p.logger.Debug("publishing commit", "subject", SubjectCommit, "sha", commit.SHA, "repo_id", commit.RepoID)
	return publish(ctx, p.stream, p.logger, SubjectCommit, commit)
}

func (p *GITPublisher) PublishPullRequest(ctx context.Context, pr model.PullRequest) error {
	p.logger.Debug("publishing pull request", "subject", SubjectPullRequest, "id", pr.ID)
	return publish(ctx, p.stream, p.logger, SubjectPullRequest, pr)
}
