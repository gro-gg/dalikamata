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

// PullRequestSubscriber is the incoming port for receiving pull request events.
type PullRequestSubscriber interface {
	Subscribe(ctx context.Context, handler func(model.PullRequest)) error
}

// Repository is the secondary (driven) port for persisting git entities.
type Repository interface {
	AddRepo(context.Context, model.Repo) error
	AddCommit(context.Context, model.Commit) error
	AddPullRequest(context.Context, model.PullRequest) error
}

// GitEventHandler is the primary (driving) port the NATS adapter calls into.
type GitEventHandler interface {
	HandleRepo(context.Context, model.Repo) error
	HandleCommit(context.Context, model.Commit) error
	HandlePullRequest(context.Context, model.PullRequest) error
}

// Pipeline is the secondary (driven) port for persisting pipeline entities.
type Pipeline interface {
	AddJob(context.Context, model.Job) error
}

// PipelineEventHandler is the primary (driving) port the NATS adapter calls into.
type PipelineEventHandler interface {
	HandleJob(context.Context, model.Job) error
}

// PipelinePublisher is the outgoing port for emitting CI/CD pipeline events.
type PipelinePublisher interface {
	PublishJob(context.Context, model.Job) error
	PublishBuild(context.Context, model.Build) error
	PublishPipelineStage(context.Context, model.PipelineStage) error
}
