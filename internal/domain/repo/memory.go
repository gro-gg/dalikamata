package repo

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// MemoryRepositoryOpt configures a MemoryRepository.
type MemoryRepositoryOpt func(*MemoryRepository)

// WithClock overrides the clock used to compute cycle_time_seconds for OPEN PRs.
// Useful in tests to produce deterministic results.
func WithClock(clock func() time.Time) MemoryRepositoryOpt {
	return func(r *MemoryRepository) { r.clock = clock }
}

type MemoryRepository struct {
	mu            sync.RWMutex
	repos         map[string]model.Repo
	commits       map[string]model.Commit
	pullRequests  map[string]model.PullRequest
	workflows     map[string]model.Workflow
	workflowRuns  map[string]model.WorkflowRun
	workflowTasks map[string]model.WorkflowTask
	teams         map[string]model.Team
	components    map[string]model.Component
	clock         func() time.Time
}

func NewMemory(opts ...MemoryRepositoryOpt) *MemoryRepository {
	r := &MemoryRepository{
		repos:         make(map[string]model.Repo),
		commits:       make(map[string]model.Commit),
		pullRequests:  make(map[string]model.PullRequest),
		workflows:     make(map[string]model.Workflow),
		workflowRuns:  make(map[string]model.WorkflowRun),
		workflowTasks: make(map[string]model.WorkflowTask),
		teams:         make(map[string]model.Team),
		components:    make(map[string]model.Component),
		clock:         time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	return r
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

func (r *MemoryRepository) AddTeam(_ context.Context, team model.Team) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams[team.Name] = team
	return nil
}

func (r *MemoryRepository) AddComponent(_ context.Context, comp model.Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.components[comp.Name] = comp
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
	now := r.clock()
	return queryEntities(ctx, snapshot, q, func(pr model.PullRequest) map[string]any {
		return projectPullRequest(pr, now)
	}, emit)
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

func (r *MemoryRepository) QueryTeams(ctx context.Context, q query.Query, emit func(model.Team) error) error {
	r.mu.RLock()
	snapshot := make([]model.Team, 0, len(r.teams))
	for _, v := range r.teams {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectTeam, emit)
}

func (r *MemoryRepository) QueryComponents(ctx context.Context, q query.Query, emit func(model.Component) error) error {
	r.mu.RLock()
	snapshot := make([]model.Component, 0, len(r.components))
	for _, v := range r.components {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, projectComponent, emit)
}

// Aggregate applies the aggregation tree in q.Aggs to all entities of q.Entity
// that match q.Filter, returning the named aggregation results.
// Returns nil when q.Aggs is empty.
func (r *MemoryRepository) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	if len(q.Aggs) == 0 {
		return nil, nil
	}
	now := r.clock()
	projected, err := r.snapshotProject(ctx, q.Entity, q.Filter, now)
	if err != nil {
		return nil, err
	}
	return query.EvaluateAggs(projected, q.Aggs)
}

// snapshotProject takes a read-locked snapshot of the named entity collection,
// releases the lock, then filters and projects each item to map[string]any.
func (r *MemoryRepository) snapshotProject(ctx context.Context, entity query.Entity, filter *query.Filter, now time.Time) ([]map[string]any, error) {
	switch entity {
	case query.EntityRepo:
		r.mu.RLock()
		snap := make([]model.Repo, 0, len(r.repos))
		for _, v := range r.repos {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.Repo) map[string]any { return projectRepo(v) })

	case query.EntityCommit:
		r.mu.RLock()
		snap := make([]model.Commit, 0, len(r.commits))
		for _, v := range r.commits {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.Commit) map[string]any { return projectCommit(v) })

	case query.EntityPullRequest:
		r.mu.RLock()
		snap := make([]model.PullRequest, 0, len(r.pullRequests))
		for _, v := range r.pullRequests {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.PullRequest) map[string]any { return projectPullRequest(v, now) })

	case query.EntityWorkflow:
		r.mu.RLock()
		snap := make([]model.Workflow, 0, len(r.workflows))
		for _, v := range r.workflows {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.Workflow) map[string]any { return projectWorkflow(v) })

	case query.EntityWorkflowRun:
		r.mu.RLock()
		snap := make([]model.WorkflowRun, 0, len(r.workflowRuns))
		for _, v := range r.workflowRuns {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.WorkflowRun) map[string]any { return projectWorkflowRun(v) })

	case query.EntityWorkflowTask:
		r.mu.RLock()
		snap := make([]model.WorkflowTask, 0, len(r.workflowTasks))
		for _, v := range r.workflowTasks {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.WorkflowTask) map[string]any { return projectWorkflowTask(v) })

	case query.EntityTeam:
		r.mu.RLock()
		snap := make([]model.Team, 0, len(r.teams))
		for _, v := range r.teams {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.Team) map[string]any { return projectTeam(v) })

	case query.EntityComponent:
		r.mu.RLock()
		snap := make([]model.Component, 0, len(r.components))
		for _, v := range r.components {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.Component) map[string]any { return projectComponent(v) })

	default:
		return nil, fmt.Errorf("unknown entity: %q", entity)
	}
}

// filterProject filters snapshot items by filter and projects matching ones to
// map[string]any. Respects context cancellation.
func filterProject[T any](
	ctx context.Context,
	snapshot []T,
	filter *query.Filter,
	project func(T) map[string]any,
) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(snapshot))
	for _, item := range snapshot {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := project(item)
		ok, err := query.Match(filter, fields)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, fields)
		}
	}
	return result, nil
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
