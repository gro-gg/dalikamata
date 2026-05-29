package domain

import (
	"context"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// GitEventHandler is the primary (driving) port the NATS adapter calls into.
type GitEventHandler interface {
	HandleRepo(context.Context, model.Repo) error
	HandleCommit(context.Context, model.Commit) error
	HandlePullRequest(context.Context, model.PullRequest) error
}

// CicdEventHandler is the primary (driving) port the NATS adapter calls into.
type CicdEventHandler interface {
	HandleWorkflow(context.Context, model.Workflow) error
	HandleWorkflowRun(context.Context, model.WorkflowRun) error
	HandleWorkflowTask(context.Context, model.WorkflowTask) error
}

// PlatformEventHandler is the primary (driving) port for platform config events.
type PlatformEventHandler interface {
	HandleTeam(context.Context, model.Team) error
	HandleComponent(context.Context, model.Component) error
}

// QueryHandler is the primary (driving) port the NATS query adapter calls into.
type QueryHandler interface {
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

type DomainService struct {
	repo      Repository
	queryRepo QueryRepository
	logger    *slog.Logger
}

func NewDomainService(repo Repository, queryRepo QueryRepository, logger *slog.Logger) *DomainService {
	return &DomainService{
		repo:      repo,
		queryRepo: queryRepo,
		logger:    logger.With("component", "domain_service"),
	}
}

func (s *DomainService) HandleRepo(ctx context.Context, r model.Repo) error {
	s.logger.Info("handling repo", "repo_id", r.RepoID)
	return s.repo.AddRepo(ctx, r)
}

func (s *DomainService) HandleCommit(ctx context.Context, c model.Commit) error {
	s.logger.Info("handling commit", "sha", c.SHA, "repo_id", c.RepoID)
	return s.repo.AddCommit(ctx, c)
}

func (s *DomainService) HandlePullRequest(ctx context.Context, pr model.PullRequest) error {
	s.logger.Info("handling pull request", "id", pr.ID, "repo_id", pr.RepoID)
	return s.repo.AddPullRequest(ctx, pr)
}

func (s *DomainService) HandleWorkflow(ctx context.Context, workflow model.Workflow) error {
	s.logger.Info("handling workflow", "id", workflow.ID)
	return s.repo.AddWorkflow(ctx, workflow)
}

func (s *DomainService) HandleWorkflowRun(ctx context.Context, workflowRun model.WorkflowRun) error {
	s.logger.Info("handling workflow run", "id", workflowRun.ID, "workflow_id", workflowRun.WorkflowID)
	return s.repo.AddWorkflowRun(ctx, workflowRun)
}

func (s *DomainService) HandleWorkflowTask(ctx context.Context, workflowTask model.WorkflowTask) error {
	s.logger.Info("handling pipeline workflow task", "id", workflowTask.WorkflowRunID, "name", workflowTask.Name)
	return s.repo.AddWorkflowTask(ctx, workflowTask)
}

func (s *DomainService) HandleTeam(ctx context.Context, team model.Team) error {
	s.logger.Info("handling team", "name", team.Name)
	return s.repo.AddTeam(ctx, team)
}

func (s *DomainService) HandleComponent(ctx context.Context, comp model.Component) error {
	s.logger.Info("handling component", "name", comp.Name, "team", comp.TeamName)
	return s.repo.AddComponent(ctx, comp)
}

func (s *DomainService) QueryRepos(ctx context.Context, q query.Query, emit func(model.Repo) error) error {
	return s.queryRepo.QueryRepos(ctx, q, emit)
}

func (s *DomainService) QueryCommits(ctx context.Context, q query.Query, emit func(model.Commit) error) error {
	return s.queryRepo.QueryCommits(ctx, q, emit)
}

func (s *DomainService) QueryPullRequests(ctx context.Context, q query.Query, emit func(model.PullRequest) error) error {
	return s.queryRepo.QueryPullRequests(ctx, q, emit)
}

func (s *DomainService) QueryWorkflows(ctx context.Context, q query.Query, emit func(model.Workflow) error) error {
	return s.queryRepo.QueryWorkflows(ctx, q, emit)
}

func (s *DomainService) QueryWorkflowRuns(ctx context.Context, q query.Query, emit func(model.WorkflowRun) error) error {
	return s.queryRepo.QueryWorkflowRuns(ctx, q, emit)
}

func (s *DomainService) QueryWorkflowTasks(ctx context.Context, q query.Query, emit func(model.WorkflowTask) error) error {
	return s.queryRepo.QueryWorkflowTasks(ctx, q, emit)
}

func (s *DomainService) QueryTeams(ctx context.Context, q query.Query, emit func(model.Team) error) error {
	return s.queryRepo.QueryTeams(ctx, q, emit)
}

func (s *DomainService) QueryComponents(ctx context.Context, q query.Query, emit func(model.Component) error) error {
	return s.queryRepo.QueryComponents(ctx, q, emit)
}

func (s *DomainService) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	return s.queryRepo.Aggregate(ctx, q)
}
