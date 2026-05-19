package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

type PipelinePublisher struct {
	stream jetstream.JetStream
	logger *slog.Logger
}

func NewPipelinePublisher(ctx context.Context, natsURL string, logger *slog.Logger) (domain.PipelinePublisher, func(), error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline publisher connecting to NATS: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline publisher connecting to JetStream: %w", err)
	}

	p := &PipelinePublisher{
		stream: js,
		logger: logger,
	}

	return p, func() { nc.Close() }, nil
}

func (p *PipelinePublisher) PublishJob(ctx context.Context, job model.Job) error {
	p.logger.Debug("publishing job", "subject", SubjectPipelineJob, "job_id", job.JobID)
	return p.publish(ctx, SubjectPipelineJob, job)
}

func (p *PipelinePublisher) PublishBuild(ctx context.Context, build model.Build) error {
	return p.publish(ctx, SubjectPipelineBuild, build)
}

func (p *PipelinePublisher) PublishPipelineStage(ctx context.Context, stage model.PipelineStage) error {
	return p.publish(ctx, SubjectPipelineStage, stage)
}

func (p *PipelinePublisher) publish(ctx context.Context, subject string, payload any) error {
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
