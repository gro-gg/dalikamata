package repo

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
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
	// Reverse indexes maintained by AddComponent so that WorkflowRun/Task
	// projections can surface team_name and component_name without a join.
	repoToComponent map[string]string // repoID → component name
	componentToTeam map[string]string // component name → team name
	clock           func() time.Time
}

func NewMemory(opts ...MemoryRepositoryOpt) *MemoryRepository {
	r := &MemoryRepository{
		repos:           make(map[string]model.Repo),
		commits:         make(map[string]model.Commit),
		pullRequests:    make(map[string]model.PullRequest),
		workflows:       make(map[string]model.Workflow),
		workflowRuns:    make(map[string]model.WorkflowRun),
		workflowTasks:   make(map[string]model.WorkflowTask),
		teams:           make(map[string]model.Team),
		components:      make(map[string]model.Component),
		repoToComponent: make(map[string]string),
		componentToTeam: make(map[string]string),
		clock:           time.Now,
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
	r.workflowTasks[fmt.Sprintf("%s/%d", stage.WorkflowRunID, stage.Order)] = stage
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
	// Remove stale repo→component entries from the previous version of
	// this component (handles re-ingests where the repo list shrinks).
	if prev, ok := r.components[comp.Name]; ok {
		for _, repoID := range prev.RepoIDs {
			if r.repoToComponent[repoID] == prev.Name {
				delete(r.repoToComponent, repoID)
			}
		}
	}
	r.components[comp.Name] = comp
	r.componentToTeam[comp.Name] = comp.TeamName
	for _, repoID := range comp.RepoIDs {
		r.repoToComponent[repoID] = comp.Name
	}
	return nil
}

// AddRepoOnboarding applies a per-repo self-onboarding event (ADR-007). It
// upserts the team, reassigns the repo to the named component (removing it from
// any other component it previously belonged to), and updates the ownership
// indexes. It is idempotent: re-applying the same event is a no-op.
func (r *MemoryRepository) AddRepoOnboarding(_ context.Context, o model.RepoOnboarding) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Upsert the owning team.
	r.teams[o.Team] = model.Team{Name: o.Team}

	// A repo belongs to at most one component: remove it from any other
	// component that currently lists it.
	if prevName, ok := r.repoToComponent[o.RepoID]; ok && prevName != o.Component {
		if prev, ok := r.components[prevName]; ok {
			prev.RepoIDs = slices.DeleteFunc(prev.RepoIDs, func(id string) bool { return id == o.RepoID })
			r.components[prevName] = prev
		}
	}

	// Upsert the target component, set its team, and ensure it lists the repo.
	comp := r.components[o.Component]
	comp.Name = o.Component
	comp.TeamName = o.Team
	if !slices.Contains(comp.RepoIDs, o.RepoID) {
		comp.RepoIDs = append(comp.RepoIDs, o.RepoID)
	}
	r.components[o.Component] = comp

	r.componentToTeam[o.Component] = o.Team
	r.repoToComponent[o.RepoID] = o.Component
	return nil
}

// snapshotOwnerLookup captures a point-in-time copy of the indexes and workflow
// names under a read lock and returns an ownerLookup whose closures operate on
// the snapshot — safe to call after the lock is released.
func (r *MemoryRepository) snapshotOwnerLookup() ownerLookup {
	rtc := make(map[string]string, len(r.repoToComponent))
	for k, v := range r.repoToComponent {
		rtc[k] = v
	}
	ct := make(map[string]string, len(r.componentToTeam))
	for k, v := range r.componentToTeam {
		ct[k] = v
	}
	wtr := make(map[string]string, len(r.workflows))
	wfNames := make(map[string]string, len(r.workflows))
	for k, v := range r.workflows {
		wtr[k] = v.RepoID
		wfNames[k] = v.Name
	}
	return newOwnerLookup(rtc, wtr, ct, wfNames)
}

// newOwnerLookup builds an ownerLookup from four plain maps:
//   - rtc:     repoID → owning component name
//   - wtr:     workflowID → repoID
//   - ct:      component name → team name
//   - wfNames: workflowID → human-readable workflow name
//
// Ownership is derived as: workflowID → repoID → componentName → teamName.
// The returned closures only read these maps, so callers must pass copies they
// own (no shared mutable state) to keep the lookup safe to use without a lock.
func newOwnerLookup(rtc, wtr, ct, wfNames map[string]string) ownerLookup {
	return ownerLookup{
		ownership: func(workflowID string) (string, string) {
			repoID := wtr[workflowID]
			compName := rtc[repoID]
			if compName == "" {
				return "unknown", "unknown"
			}
			teamName, ok := ct[compName]
			if !ok {
				return compName, "unknown"
			}
			return compName, teamName
		},
		workflowName: func(workflowID string) string {
			if name, ok := wfNames[workflowID]; ok {
				return name
			}
			return workflowID
		},
		diagnose: func(workflowID string) model.OwnershipDiagnostics {
			repoID := wtr[workflowID]
			if repoID == "" {
				return model.OwnershipDiagnostics{WorkflowID: workflowID, Reason: "missing_repo_id"}
			}
			compName := rtc[repoID]
			if compName == "" {
				return model.OwnershipDiagnostics{WorkflowID: workflowID, RepoID: repoID, Reason: "no_component_for_repo"}
			}
			teamName := ct[compName]
			if teamName == "" {
				return model.OwnershipDiagnostics{WorkflowID: workflowID, RepoID: repoID, ComponentName: compName, Reason: "no_team_for_component"}
			}
			return model.OwnershipDiagnostics{WorkflowID: workflowID, RepoID: repoID, ComponentName: compName, TeamName: teamName, Reason: "ok"}
		},
	}
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
	lkp := r.snapshotOwnerLookup()
	r.mu.RUnlock()
	return queryEntities(ctx, snapshot, q, func(run model.WorkflowRun) map[string]any {
		return projectWorkflowRun(run, lkp)
	}, func(run model.WorkflowRun) error {
		run.WorkflowName = lkp.workflowName(run.WorkflowID)
		run.ComponentName, run.TeamName = lkp.ownership(run.WorkflowID)
		return emit(run)
	})
}

func (r *MemoryRepository) QueryWorkflowTasks(ctx context.Context, q query.Query, emit func(model.WorkflowTask) error) error {
	r.mu.RLock()
	snapTasks := make([]model.WorkflowTask, 0, len(r.workflowTasks))
	for _, v := range r.workflowTasks {
		snapTasks = append(snapTasks, v)
	}
	snapRuns := make(map[string]model.WorkflowRun, len(r.workflowRuns))
	for k, v := range r.workflowRuns {
		snapRuns[k] = v
	}
	lkp := r.snapshotOwnerLookup()
	r.mu.RUnlock()
	return queryEntities(ctx, snapTasks, q, func(t model.WorkflowTask) map[string]any {
		return projectWorkflowTask(t, snapRuns, lkp)
	}, func(t model.WorkflowTask) error {
		if run, ok := snapRuns[t.WorkflowRunID]; ok {
			t.WorkflowID = run.WorkflowID
			t.Branch = run.Branch
		}
		t.WorkflowName = lkp.workflowName(t.WorkflowID)
		t.ComponentName, t.TeamName = lkp.ownership(t.WorkflowID)
		return emit(t)
	})
}

func (r *MemoryRepository) QueryTeams(ctx context.Context, q query.Query, emit func(model.Team) error) error {
	r.mu.RLock()
	snapshot := make([]model.Team, 0, len(r.teams))
	for _, v := range r.teams {
		snapshot = append(snapshot, v)
	}
	r.mu.RUnlock()
	return queryEntities(ctx, ensureUnknownTeam(snapshot), q, projectTeam, emit)
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

func (r *MemoryRepository) OwnershipDiagnostics(_ context.Context) ([]model.OwnershipDiagnostics, error) {
	r.mu.RLock()
	ids := make([]string, 0, len(r.workflows))
	for id := range r.workflows {
		ids = append(ids, id)
	}
	lkp := r.snapshotOwnerLookup()
	r.mu.RUnlock()
	out := make([]model.OwnershipDiagnostics, len(ids))
	for i, id := range ids {
		out[i] = lkp.diagnose(id)
	}
	return out, nil
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
		lkp := r.snapshotOwnerLookup()
		r.mu.RUnlock()
		return filterProject(ctx, snap, filter, func(v model.WorkflowRun) map[string]any { return projectWorkflowRun(v, lkp) })

	case query.EntityWorkflowTask:
		r.mu.RLock()
		snapTasks := make([]model.WorkflowTask, 0, len(r.workflowTasks))
		for _, v := range r.workflowTasks {
			snapTasks = append(snapTasks, v)
		}
		snapRuns := make(map[string]model.WorkflowRun, len(r.workflowRuns))
		for k, v := range r.workflowRuns {
			snapRuns[k] = v
		}
		lkp := r.snapshotOwnerLookup()
		r.mu.RUnlock()
		return filterProject(ctx, snapTasks, filter, func(v model.WorkflowTask) map[string]any { return projectWorkflowTask(v, snapRuns, lkp) })

	case query.EntityTeam:
		r.mu.RLock()
		snap := make([]model.Team, 0, len(r.teams))
		for _, v := range r.teams {
			snap = append(snap, v)
		}
		r.mu.RUnlock()
		return filterProject(ctx, ensureUnknownTeam(snap), filter, func(v model.Team) map[string]any { return projectTeam(v) })

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
