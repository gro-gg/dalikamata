package domain

import (
	"context"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
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

// PlatformPublisher is the outgoing port for emitting platform config events.
type PlatformPublisher interface {
	PublishTeam(context.Context, model.Team) error
	PublishComponent(context.Context, model.Component) error
}

// Repository is the secondary (driven) port for persisting entities.
type Repository interface {
	AddRepo(context.Context, model.Repo) error
	AddCommit(context.Context, model.Commit) error
	AddPullRequest(context.Context, model.PullRequest) error
	AddWorkflow(context.Context, model.Workflow) error
	AddWorkflowRun(context.Context, model.WorkflowRun) error
	AddWorkflowTask(context.Context, model.WorkflowTask) error
	AddTeam(context.Context, model.Team) error
	AddComponent(context.Context, model.Component) error
}

// QueryRepository is the secondary (driven) port for querying entities.
// Each QueryX method applies the filter/sort/pagination in q and calls emit
// once per matching result. The emit callback returning an error stops
// iteration and that error is returned to the caller.
// Aggregate applies q.Aggs to filtered items and returns the result tree.
type QueryRepository interface {
	QueryRepos(ctx context.Context, q query.Query, emit func(model.Repo) error) error
	QueryCommits(ctx context.Context, q query.Query, emit func(model.Commit) error) error
	QueryPullRequests(ctx context.Context, q query.Query, emit func(model.PullRequest) error) error
	QueryWorkflows(ctx context.Context, q query.Query, emit func(model.Workflow) error) error
	QueryWorkflowRuns(ctx context.Context, q query.Query, emit func(model.WorkflowRun) error) error
	QueryWorkflowTasks(ctx context.Context, q query.Query, emit func(model.WorkflowTask) error) error
	QueryTeams(ctx context.Context, q query.Query, emit func(model.Team) error) error
	QueryComponents(ctx context.Context, q query.Query, emit func(model.Component) error) error
	Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error)
}
