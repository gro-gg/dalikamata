package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gonats "github.com/nats-io/nats.go"
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
	p.logger.Debug("publishing workflow", "subject", SubjectCicdWorkflow, "id", workflow.ID, "payload", workflow)
	return p.publish(ctx, SubjectCicdWorkflow, workflow)
}

func (p *PipelinePublisher) PublishWorkflowRun(ctx context.Context, run model.WorkflowRun) error {
	p.logger.Debug("publishing workflow run", "subject", SubjectCicdWorkflowRun, "id", run.ID, "payload", run)
	return p.publish(ctx, SubjectCicdWorkflowRun, run)
}

func (p *PipelinePublisher) PublishWorkflowTask(ctx context.Context, task model.WorkflowTask) error {
	p.logger.Debug("publishing workflow task", "subject", SubjectCicdWorkflowTask, "id", task.WorkflowRunID, "payload", task)
	return p.publish(ctx, SubjectCicdWorkflowTask, task)
}

func (p *PipelinePublisher) publish(ctx context.Context, subject string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON marshal: %w", err)
	}

	for {
		_, err = p.stream.Publish(ctx, subject, b)
		if err == nil {
			return nil
		}
		if !isStreamNotReady(err) {
			return fmt.Errorf("publishing to %s: %w", subject, err)
		}
		p.logger.Debug("INGEST stream not ready, retrying publish", "subject", subject)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// isStreamNotReady reports whether the publish error is a transient condition
// caused by the INGEST stream not existing yet (e.g. domain service still
// starting up). Both ErrNoResponders and a 404 JetStream APIError indicate
// this state.
func isStreamNotReady(err error) bool {
	if errors.Is(err, gonats.ErrNoResponders) || errors.Is(err, jetstream.ErrNoStreamResponse) {
		return true
	}
	var apiErr *jetstream.APIError
	return errors.As(err, &apiErr) && apiErr.Code == 404
}
