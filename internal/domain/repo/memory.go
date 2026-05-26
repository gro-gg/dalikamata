package repo

import (
	"context"
	"sync"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

type MemoryRepository struct {
	mu            sync.RWMutex
	repos         map[string]model.Repo
	commits       map[string]model.Commit
	pullRequests  map[string]model.PullRequest
	workflows     map[string]model.Workflow
	workflowRuns  map[string]model.WorkflowRun
	workflowTasks map[string]model.WorkflowTask
}

func NewMemory() *MemoryRepository {
	return &MemoryRepository{
		repos:         make(map[string]model.Repo),
		commits:       make(map[string]model.Commit),
		pullRequests:  make(map[string]model.PullRequest),
		workflows:     make(map[string]model.Workflow),
		workflowRuns:  make(map[string]model.WorkflowRun),
		workflowTasks: make(map[string]model.WorkflowTask),
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

func (r *MemoryRepository) AddWorkflow(_ context.Context, job model.Workflow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflows[job.ID] = job
	return nil
}

func (r *MemoryRepository) AddWorkflowRun(_ context.Context, build model.WorkflowRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflowRuns[build.ID] = build
	return nil
}

func (r *MemoryRepository) AddWorkflowTask(_ context.Context, stage model.WorkflowTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflowTasks[stage.ID+"/"+stage.Name] = stage
	return nil
}
