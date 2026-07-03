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

type PipelinePublisher struct {
	stream jetstream.JetStream
	logger *slog.Logger
}

func NewPipelinePublisher(ctx context.Context, natsURL string, logger *slog.Logger) (domain.CICDPublisher, func(), error) {
	_, js, closeConn, err := nats.Connect(ctx, natsURL, logger)
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
	return publish(ctx, p.stream, p.logger, SubjectCicdWorkflow, workflow)
}

func (p *PipelinePublisher) PublishWorkflowRun(ctx context.Context, run model.WorkflowRun) error {
	p.logger.Debug("publishing workflow run", "subject", SubjectCicdWorkflowRun, "id", run.ID)
	return publish(ctx, p.stream, p.logger, SubjectCicdWorkflowRun, run)
}

func (p *PipelinePublisher) PublishWorkflowTask(ctx context.Context, task model.WorkflowTask) error {
	p.logger.Debug("publishing workflow task", "subject", SubjectCicdWorkflowTask, "workflow_run_id", task.WorkflowRunID)
	return publish(ctx, p.stream, p.logger, SubjectCicdWorkflowTask, task)
}
