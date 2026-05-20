package domain

import (
	"context"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

type Service interface {
	Run(context.Context) error
}

type DomainService struct {
	repo     Repository
	pipeline Pipeline
	logger   *slog.Logger
}

func NewDomainService(repo Repository, pipeline Pipeline, logger *slog.Logger) *DomainService {
	return &DomainService{
		repo:     repo,
		pipeline: pipeline,
		logger:   logger.With("component", "domain_service"),
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

func (s *DomainService) HandleJob(ctx context.Context, job model.Job) error {
	s.logger.Info("handling job", "job_id", job.JobID)
	return s.pipeline.AddJob(ctx, job)
}
