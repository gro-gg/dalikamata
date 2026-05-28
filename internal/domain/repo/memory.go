package repo

import (
	"context"
	"sync"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
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
	r.workflowTasks[stage.WorkflowRunID+"/"+stage.Name] = stage
	return nil
}

func (r *MemoryRepository) QueryRepos(ctx context.Context, q query.Query, emit func(model.Repo) error) error {
	r.mu.RLock()
	snapshot := make([]model.Repo, 0, len(r.repos))
	for _, v := range r.repos {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectRepo, emit)
}

func (r *MemoryRepository) QueryCommits(ctx context.Context, q query.Query, emit func(model.Commit) error) error {
	r.mu.RLock()
	snapshot := make([]model.Commit, 0, len(r.commits))
	for _, v := range r.commits {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectCommit, emit)
}

func (r *MemoryRepository) QueryPullRequests(ctx context.Context, q query.Query, emit func(model.PullRequest) error) error {
	r.mu.RLock()
	snapshot := make([]model.PullRequest, 0, len(r.pullRequests))
	for _, v := range r.pullRequests {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectPullRequest, emit)
}

func (r *MemoryRepository) QueryWorkflows(ctx context.Context, q query.Query, emit func(model.Workflow) error) error {
	r.mu.RLock()
	snapshot := make([]model.Workflow, 0, len(r.workflows))
	for _, v := range r.workflows {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectWorkflow, emit)
}

func (r *MemoryRepository) QueryWorkflowRuns(ctx context.Context, q query.Query, emit func(model.WorkflowRun) error) error {
	r.mu.RLock()
	snapshot := make([]model.WorkflowRun, 0, len(r.workflowRuns))
	for _, v := range r.workflowRuns {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectWorkflowRun, emit)
}

func (r *MemoryRepository) QueryWorkflowTasks(ctx context.Context, q query.Query, emit func(model.WorkflowTask) error) error {
	r.mu.RLock()
	snapshot := make([]model.WorkflowTask, 0, len(r.workflowTasks))
	for _, v := range r.workflowTasks {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectWorkflowTask, emit)
}

// queryEntities is the shared implementation for all QueryX methods.
// It filters, sorts, paginates, and emits each matching entity.
// The snapshot copy under RLock ensures writers are never blocked by consumers.
func queryEntities[T any](
	ctx context.Context,
	snapshot []T,
	q query.Query,
	project func(T) map[string]any,
	emit func(T) error,
) error {
	filtered := snapshot[:0:0]
	for _, item := range snapshot {
		if err := ctx.Err(); err != nil {
			return err
		}
		ok, err := query.Match(q.Filter, project(item))
		if err != nil {
			return err
		}
		if ok {
			filtered = append(filtered, item)
		}
	}
	query.SortBy(filtered, q.Sort, project)
	for _, item := range query.Paginate(filtered, q.From, q.Size) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(item); err != nil {
			return err
		}
	}
	return nil
}
