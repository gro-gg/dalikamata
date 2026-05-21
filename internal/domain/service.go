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

func (s *DomainService) HandleJob(ctx context.Context, job model.Job) error {
	s.logger.Info("handling job", "job_id", job.JobID)
	return s.repo.AddJob(ctx, job)
}

func (s *DomainService) HandleBuild(ctx context.Context, build model.Build) error {
	s.logger.Info("handling build", "build_id", build.ID, "job_id", build.JobID)
	return s.repo.AddBuild(ctx, build)
}

func (s *DomainService) HandlePipelineStage(ctx context.Context, stage model.PipelineStage) error {
	s.logger.Info("handling pipeline stage", "build_id", stage.BuildID, "name", stage.Name)
	return s.repo.AddPipelineStage(ctx, stage)
}
