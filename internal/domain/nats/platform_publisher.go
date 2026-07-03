package nats

import (
	"context"
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	natsclient "codeberg.org/aeforged/dalikamata/internal/nats"
)

type PlatformPublisher struct {
	stream    jetstream.JetStream
	logger    *slog.Logger
	closeConn func()
}

func NewPlatformPublisher(ctx context.Context, natsURL string, logger *slog.Logger) (domain.PlatformPublisher, func(), error) {
	_, js, closeConn, err := natsclient.Connect(ctx, natsURL, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("platform publisher connecting to NATS: %w", err)
	}
	logger.Info("Connected to NATS")

	p := &PlatformPublisher{
		logger:    logger,
		stream:    js,
		closeConn: closeConn,
	}
	return p, p.Close, nil
}

func (p *PlatformPublisher) Close() {
	p.closeConn()
}

func (p *PlatformPublisher) PublishTeam(ctx context.Context, team model.Team) error {
	p.logger.Debug("publishing team", "subject", SubjectPlatformTeam, "name", team.Name)
	return publish(ctx, p.stream, p.logger, SubjectPlatformTeam, team)
}

func (p *PlatformPublisher) PublishComponent(ctx context.Context, comp model.Component) error {
	p.logger.Debug("publishing component", "subject", SubjectPlatformComponent, "name", comp.Name, "team", comp.TeamName)
	return publish(ctx, p.stream, p.logger, SubjectPlatformComponent, comp)
}

func (p *PlatformPublisher) PublishRepoOnboarding(ctx context.Context, repo model.RepoOnboarding) error {
	p.logger.Debug("publishing repo onboarding", "subject", SubjectPlatformRepo, "repo_id", repo.RepoID, "component", repo.Component, "team", repo.Team)
	return publish(ctx, p.stream, p.logger, SubjectPlatformRepo, repo)
}
