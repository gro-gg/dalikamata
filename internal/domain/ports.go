package domain

import (
	"context"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// GitPublisher is the outgoing port for emitting Git events.
type GitPublisher interface {
	PublishCommit(context.Context, model.Commit) error
	PublishPullRequest(context.Context, model.PullRequest) error
	PublishRepo(context.Context, model.Repo) error
}

// CICDPublisher is the outgoing port for emitting CI/CD pipeline events.
type CICDPublisher interface {
	PublishWorkflow(context.Context, model.Workflow) error
	PublishWorkflowRun(context.Context, model.WorkflowRun) error
	PublishWorkflowTask(context.Context, model.WorkflowTask) error
}

// Repository is the secondary (driven) port for persisting entities.
type Repository interface {
	AddRepo(context.Context, model.Repo) error
	AddCommit(context.Context, model.Commit) error
	AddPullRequest(context.Context, model.PullRequest) error
	AddWorkflow(context.Context, model.Workflow) error
	AddWorkflowRun(context.Context, model.WorkflowRun) error
	AddWorkflowTask(context.Context, model.WorkflowTask) error
}
