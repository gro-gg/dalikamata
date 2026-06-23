package nats

import (
	"context"
	"encoding/json"
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
	return p.publish(ctx, SubjectPlatformTeam, team)
}

func (p *PlatformPublisher) PublishComponent(ctx context.Context, comp model.Component) error {
	p.logger.Debug("publishing component", "subject", SubjectPlatformComponent, "name", comp.Name)
	return p.publish(ctx, SubjectPlatformComponent, comp)
}

func (p *PlatformPublisher) publish(ctx context.Context, subject string, payload any) error {
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
