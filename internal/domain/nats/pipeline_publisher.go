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

type PipelinePublisher struct {
	stream jetstream.JetStream
	logger *slog.Logger
}

func NewPipelinePublisher(ctx context.Context, natsURL string, logger *slog.Logger) (domain.CICDPublisher, func(), error) {
	js, closeConn, err := nats.Connect(ctx, natsURL, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline publisher connecting to NATS: %w", err)
	}

	p := &PipelinePublisher{
		stream: js,
		logger: logger,
	}

	return p, closeConn, nil
}

func (p *PipelinePublisher) PublishWorkflow(ctx context.Context, workflow model.Workflow) error {
	p.logger.Debug("publishing workflow", "subject", SubjectCicdWorkflow, "id", workflow.ID)
	return p.publish(ctx, SubjectCicdWorkflow, workflow)
}

func (p *PipelinePublisher) PublishWorkflowRun(ctx context.Context, build model.WorkflowRun) error {
	return p.publish(ctx, SubjectCicdWorkflowRun, build)
}

func (p *PipelinePublisher) PublishWorkflowTask(ctx context.Context, stage model.WorkflowTask) error {
	return p.publish(ctx, SubjectCicdWorkflowTask, stage)
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
