package repo

import (
	"context"
	"sync"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

type MemoryRepository struct {
	mu           sync.RWMutex
	repos        map[string]model.Repo
	commits      map[string]model.Commit
	pullRequests map[string]model.PullRequest
	jobs         map[string]model.Job
	builds       map[string]model.Build
	stages       map[string]model.PipelineStage
}

func NewMemory() *MemoryRepository {
	return &MemoryRepository{
		repos:        make(map[string]model.Repo),
		commits:      make(map[string]model.Commit),
		pullRequests: make(map[string]model.PullRequest),
		jobs:         make(map[string]model.Job),
		builds:       make(map[string]model.Build),
		stages:       make(map[string]model.PipelineStage),
	}
}

func (r *MemoryRepository) AddRepo(_ context.Context, repo model.Repo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.repos[repo.RepoID] = repo
	return nil
}

func (r *MemoryRepository) AddCommit(_ context.Context, commit model.Commit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commits[commit.SHA] = commit
	return nil
}

func (r *MemoryRepository) AddPullRequest(_ context.Context, pr model.PullRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pullRequests[pr.ID] = pr
	return nil
}

func (r *MemoryRepository) AddJob(_ context.Context, job model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.JobID] = job
	return nil
}

func (r *MemoryRepository) AddBuild(_ context.Context, build model.Build) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builds[build.ID] = build
	return nil
}

func (r *MemoryRepository) AddPipelineStage(_ context.Context, stage model.PipelineStage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages[stage.BuildID+"/"+stage.Name] = stage
	return nil
}
