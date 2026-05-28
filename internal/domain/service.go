package domain

import (
	"context"
	"log/slog"

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

type DomainService struct {
	repo   Repository
	logger *slog.Logger
}

func NewDomainService(repo Repository, logger *slog.Logger) *DomainService {
	return &DomainService{
		repo:   repo,
		logger: logger.With("component", "domain_service"),
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
