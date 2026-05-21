package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

type GITPublisher struct {
	stream jetstream.JetStream
	logger *slog.Logger
}

func NewGitPublisher(ctx context.Context, natsURL string, logger *slog.Logger) (domain.GitPublisher, func(), error) {
	var nc *nats.Conn
	var err error
	for {
		nc, err = nats.Connect(natsURL)
		if err == nil {
			break
		}

		logger.Error("connecting to NATS", "error", err)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	logger.Info("Connected to NATS")

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("publisher connecting to JetStream: %w", err)
	}

	p := &GITPublisher{
		logger: logger,
		stream: js,
	}

	return p,
		p.Close,
		nil
}

func (p *GITPublisher) Close() {
	p.stream.Conn().Close()
}

func (p *GITPublisher) PublishCommit(ctx context.Context, commit model.Commit) error {
	return p.publish(ctx, SubjectCommit, commit)
}

func (p *GITPublisher) PublishPullRequest(ctx context.Context, pr model.PullRequest) error {
	return p.publish(ctx, SubjectPullRequest, pr)
}

func (p *GITPublisher) PublishRepo(ctx context.Context, repo model.Repo) error {
	p.logger.Debug("publishing repo", "subject", SubjectRepo, "ID", repo.RepoID, "Name", repo.Name)
	return p.publish(ctx, SubjectRepo, repo)
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
