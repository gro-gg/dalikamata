package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/nats"
	"codeberg.org/aeforged/dalikamata/pkg/model"
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
	p.logger.Debug("publishing repo", "subject", SubjectRepo, "ID", repo.RepoID, "Name", repo.Name)
	return p.publish(ctx, SubjectRepo, repo)
}

func (p *GITPublisher) PublishCommit(ctx context.Context, commit model.Commit) error {
	p.logger.Debug("publishing commit", "subject", SubjectCommit, "SHA", commit.SHA, "RepoID", commit.RepoID)
	return p.publish(ctx, SubjectCommit, commit)
}

func (p *GITPublisher) PublishPullRequest(ctx context.Context, pr model.PullRequest) error {
	p.logger.Debug("publishing pull request", "subject", SubjectPullRequest, "ID", pr.ID)
	return p.publish(ctx, SubjectPullRequest, pr)
}

func (p *GITPublisher) publish(ctx context.Context, subject string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON marshal: %w", err)
	}

	_, err = p.stream.Publish(ctx, subject, b)
	if err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}
	return nil
}
