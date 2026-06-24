package repo

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGo), registers "sqlite"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

// schema is applied on every open; all statements are idempotent so reopening
// an existing database is a no-op. Times are stored as RFC3339Nano strings
// (offset preserved); durations and run numbers as native REAL/INTEGER. The
// Component repo_ids list is stored as a JSON array.
//
//go:embed schema.sql
var schema string

// SQLiteRepository is a filesystem-backed implementation of domain.Repository
// and domain.QueryRepository using a pure-Go SQLite driver.
//
// Storage is durable; the query engine is shared with MemoryRepository: each
// QueryX/Aggregate call loads the entity's rows into model structs and feeds
// them through the same filter/sort/paginate/aggregate helpers. This trades
// per-query load cost for a single, well-tested query implementation and avoids
// holding every entity in memory between queries.
type SQLiteRepository struct {
	db    *sql.DB
	clock func() time.Time
}

// SQLiteRepositoryOpt configures a SQLiteRepository.
type SQLiteRepositoryOpt func(*SQLiteRepository)

// WithSQLiteClock overrides the clock used to compute cycle_time_seconds for
// OPEN PRs. Mirrors WithClock on MemoryRepository; useful in tests.
func WithSQLiteClock(clock func() time.Time) SQLiteRepositoryOpt {
	return func(r *SQLiteRepository) { r.clock = clock }
}

// NewSQLite opens (creating if needed) the SQLite database at path and applies
// the schema. Pass ":memory:" for an ephemeral in-process database. The caller
// must Close the returned repository.
func NewSQLite(path string, opts ...SQLiteRepositoryOpt) (*SQLiteRepository, error) {
	// Pragmas are set via the DSN so that every connection in database/sql's
	// pool inherits them. Applying them with a one-off PRAGMA Exec only
	// configures whichever pooled connection happened to run it, leaving the
	// others with the default busy_timeout=0 — which is why concurrent ingest
	// writes failed immediately with SQLITE_BUSY instead of waiting. WAL gives
	// concurrent readers alongside a single writer; busy_timeout makes writers
	// wait rather than fail when the write lock is held.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %q: %w", path, err)
	}
	if path == ":memory:" {
		// Each new connection to ":memory:" opens a *separate* database, so pin
		// the pool to a single connection to keep schema and data consistent
		// across queries.
		db.SetMaxOpenConns(1)
	}
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	r := &SQLiteRepository{db: db, clock: time.Now}
	for _, o := range opts {
		o(r)
	}
	return r, nil
}

// Close releases the underlying database handle.
func (r *SQLiteRepository) Close() error { return r.db.Close() }

func formatTime(t time.Time) string { return t.Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

// ---- writes (upsert: re-ingesting an entity overwrites it) -----------------

func (r *SQLiteRepository) AddRepo(ctx context.Context, repo model.Repo) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO repos (repo_id, name) VALUES (?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET name=excluded.name`,
		repo.RepoID, repo.Name)
	return err
}

func (r *SQLiteRepository) AddCommit(ctx context.Context, c model.Commit) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO commits (sha, repo_id, author, timestamp) VALUES (?, ?, ?, ?)
		ON CONFLICT(sha) DO UPDATE SET
			repo_id=excluded.repo_id, author=excluded.author, timestamp=excluded.timestamp`,
		c.SHA, c.RepoID, c.Author, formatTime(c.Timestamp))
	return err
}

func (r *SQLiteRepository) AddPullRequest(ctx context.Context, pr model.PullRequest) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pull_requests
			(id, repo_id, name, title, description, state, author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			repo_id=excluded.repo_id, name=excluded.name, title=excluded.title,
			description=excluded.description, state=excluded.state, author=excluded.author,
			created_at=excluded.created_at, updated_at=excluded.updated_at`,
		pr.ID, pr.RepoID, pr.Name, pr.Title, pr.Description, pr.State, pr.Author,
		formatTime(pr.CreatedAt), formatTime(pr.UpdatedAt))
	return err
}

func (r *SQLiteRepository) AddWorkflow(ctx context.Context, w model.Workflow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO workflows (id, name, repo_id) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, repo_id=excluded.repo_id`,
		w.ID, w.Name, w.RepoID)
	return err
}

func (r *SQLiteRepository) AddWorkflowRun(ctx context.Context, run model.WorkflowRun) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO workflow_runs
			(id, workflow_id, number, status, branch, commit_sha, started_at, duration)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			workflow_id=excluded.workflow_id, number=excluded.number, status=excluded.status,
			branch=excluded.branch, commit_sha=excluded.commit_sha,
			started_at=excluded.started_at, duration=excluded.duration`,
		run.ID, run.WorkflowID, run.Number, run.Status, run.Branch, run.CommitSHA,
		formatTime(run.StartedAt), run.Duration)
	return err
}

func (r *SQLiteRepository) AddWorkflowTask(ctx context.Context, t model.WorkflowTask) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO workflow_tasks
			(workflow_run_id, task_order, name, status, started_at, duration)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workflow_run_id, task_order) DO UPDATE SET
			name=excluded.name, status=excluded.status,
			started_at=excluded.started_at, duration=excluded.duration`,
		t.WorkflowRunID, t.Order, t.Name, t.Status, formatTime(t.StartedAt), t.Duration)
	return err
}

func (r *SQLiteRepository) AddTeam(ctx context.Context, team model.Team) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO teams (name) VALUES (?) ON CONFLICT(name) DO NOTHING`,
		team.Name)
	return err
}

func (r *SQLiteRepository) AddComponent(ctx context.Context, c model.Component) error {
	repoIDs, err := json.Marshal(c.RepoIDs)
	if err != nil {
		return fmt.Errorf("marshaling component repo_ids: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO components (name, team_name, repo_ids)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			team_name=excluded.team_name, repo_ids=excluded.repo_ids`,
		c.Name, c.TeamName, string(repoIDs))
	return err
}

// ---- row loaders -----------------------------------------------------------

func (r *SQLiteRepository) loadRepos(ctx context.Context) ([]model.Repo, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT repo_id, name FROM repos`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Repo
	for rows.Next() {
		var v model.Repo
		if err := rows.Scan(&v.RepoID, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadCommits(ctx context.Context) ([]model.Commit, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT sha, repo_id, author, timestamp FROM commits`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Commit
	for rows.Next() {
		var v model.Commit
		var ts string
		if err := rows.Scan(&v.SHA, &v.RepoID, &v.Author, &ts); err != nil {
			return nil, err
		}
		if v.Timestamp, err = parseTime(ts); err != nil {
			return nil, fmt.Errorf("parsing commit timestamp: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadPullRequests(ctx context.Context) ([]model.PullRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, repo_id, name, title, description, state, author, created_at, updated_at
		FROM pull_requests`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.PullRequest
	for rows.Next() {
		var v model.PullRequest
		var created, updated string
		if err := rows.Scan(&v.ID, &v.RepoID, &v.Name, &v.Title, &v.Description,
			&v.State, &v.Author, &created, &updated); err != nil {
			return nil, err
		}
		if v.CreatedAt, err = parseTime(created); err != nil {
			return nil, fmt.Errorf("parsing pull request created_at: %w", err)
		}
		if v.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, fmt.Errorf("parsing pull request updated_at: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadWorkflows(ctx context.Context) ([]model.Workflow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, repo_id FROM workflows`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Workflow
	for rows.Next() {
		var v model.Workflow
		if err := rows.Scan(&v.ID, &v.Name, &v.RepoID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadWorkflowRuns(ctx context.Context) ([]model.WorkflowRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workflow_id, number, status, branch, commit_sha, started_at, duration
		FROM workflow_runs`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.WorkflowRun
	for rows.Next() {
		var v model.WorkflowRun
		var started string
		if err := rows.Scan(&v.ID, &v.WorkflowID, &v.Number, &v.Status, &v.Branch,
			&v.CommitSHA, &started, &v.Duration); err != nil {
			return nil, err
		}
		if v.StartedAt, err = parseTime(started); err != nil {
			return nil, fmt.Errorf("parsing workflow run started_at: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadWorkflowTasks(ctx context.Context) ([]model.WorkflowTask, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT workflow_run_id, task_order, name, status, started_at, duration
		FROM workflow_tasks`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.WorkflowTask
	for rows.Next() {
		var v model.WorkflowTask
		var started string
		if err := rows.Scan(&v.WorkflowRunID, &v.Order, &v.Name, &v.Status,
			&started, &v.Duration); err != nil {
			return nil, err
		}
		if v.StartedAt, err = parseTime(started); err != nil {
			return nil, fmt.Errorf("parsing workflow task started_at: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadTeams(ctx context.Context) ([]model.Team, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM teams`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Team
	for rows.Next() {
		var v model.Team
		if err := rows.Scan(&v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) loadComponents(ctx context.Context) ([]model.Component, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name, team_name, repo_ids FROM components ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Component
	for rows.Next() {
		var v model.Component
		var repoIDs string
		if err := rows.Scan(&v.Name, &v.TeamName, &repoIDs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(repoIDs), &v.RepoIDs); err != nil {
			return nil, fmt.Errorf("unmarshaling component repo_ids: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ownerLookupFromDB rebuilds the workflow→repo→component→team ownership lookup
// from the components and workflows tables. Unlike MemoryRepository it holds no
// incremental index; the mapping is reconstructed per query from current state,
// which naturally reflects component re-ingests that shrink the repo list.
func (r *SQLiteRepository) ownerLookupFromDB(ctx context.Context) (ownerLookup, error) {
	components, err := r.loadComponents(ctx)
	if err != nil {
		return ownerLookup{}, err
	}
	workflows, err := r.loadWorkflows(ctx)
	if err != nil {
		return ownerLookup{}, err
	}
	rtc := make(map[string]string) // repoID → componentName
	ct := make(map[string]string)  // componentName → teamName
	for _, c := range components {
		ct[c.Name] = c.TeamName
		for _, repoID := range c.RepoIDs {
			rtc[repoID] = c.Name
		}
	}
	wtr := make(map[string]string, len(workflows))     // workflowID → repoID
	wfNames := make(map[string]string, len(workflows)) // workflowID → name
	for _, w := range workflows {
		wtr[w.ID] = w.RepoID
		wfNames[w.ID] = w.Name
	}
	return newOwnerLookup(rtc, wtr, ct, wfNames), nil
}

// ---- queries (delegate to the shared query engine) -------------------------

func (r *SQLiteRepository) QueryRepos(ctx context.Context, q query.Query, emit func(model.Repo) error) error {
	snapshot, err := r.loadRepos(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, snapshot, q, projectRepo, emit)
}

func (r *SQLiteRepository) QueryCommits(ctx context.Context, q query.Query, emit func(model.Commit) error) error {
	snapshot, err := r.loadCommits(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, snapshot, q, projectCommit, emit)
}

func (r *SQLiteRepository) QueryPullRequests(ctx context.Context, q query.Query, emit func(model.PullRequest) error) error {
	snapshot, err := r.loadPullRequests(ctx)
	if err != nil {
		return err
	}
	now := r.clock()
	return queryEntities(ctx, snapshot, q, func(pr model.PullRequest) map[string]any {
		return projectPullRequest(pr, now)
	}, emit)
}

func (r *SQLiteRepository) QueryWorkflows(ctx context.Context, q query.Query, emit func(model.Workflow) error) error {
	snapshot, err := r.loadWorkflows(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, snapshot, q, projectWorkflow, emit)
}

func (r *SQLiteRepository) QueryWorkflowRuns(ctx context.Context, q query.Query, emit func(model.WorkflowRun) error) error {
	snapshot, err := r.loadWorkflowRuns(ctx)
	if err != nil {
		return err
	}
	lkp, err := r.ownerLookupFromDB(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, snapshot, q, func(run model.WorkflowRun) map[string]any {
		return projectWorkflowRun(run, lkp)
	}, func(run model.WorkflowRun) error {
		run.WorkflowName = lkp.workflowName(run.WorkflowID)
		run.ComponentName, run.TeamName = lkp.ownership(run.WorkflowID)
		return emit(run)
	})
}

func (r *SQLiteRepository) QueryWorkflowTasks(ctx context.Context, q query.Query, emit func(model.WorkflowTask) error) error {
	snapTasks, err := r.loadWorkflowTasks(ctx)
	if err != nil {
		return err
	}
	runs, err := r.loadWorkflowRuns(ctx)
	if err != nil {
		return err
	}
	snapRuns := make(map[string]model.WorkflowRun, len(runs))
	for _, run := range runs {
		snapRuns[run.ID] = run
	}
	lkp, err := r.ownerLookupFromDB(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, snapTasks, q, func(t model.WorkflowTask) map[string]any {
		return projectWorkflowTask(t, snapRuns, lkp)
	}, func(t model.WorkflowTask) error {
		if run, ok := snapRuns[t.WorkflowRunID]; ok {
			t.WorkflowID = run.WorkflowID
		}
		t.WorkflowName = lkp.workflowName(t.WorkflowID)
		t.ComponentName, t.TeamName = lkp.ownership(t.WorkflowID)
		return emit(t)
	})
}

func (r *SQLiteRepository) QueryTeams(ctx context.Context, q query.Query, emit func(model.Team) error) error {
	snapshot, err := r.loadTeams(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, ensureUnknownTeam(snapshot), q, projectTeam, emit)
}

func (r *SQLiteRepository) QueryComponents(ctx context.Context, q query.Query, emit func(model.Component) error) error {
	snapshot, err := r.loadComponents(ctx)
	if err != nil {
		return err
	}
	return queryEntities(ctx, snapshot, q, projectComponent, emit)
}

func (r *SQLiteRepository) OwnershipDiagnostics(ctx context.Context) ([]model.OwnershipDiagnostics, error) {
	workflows, err := r.loadWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	lkp, err := r.ownerLookupFromDB(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.OwnershipDiagnostics, len(workflows))
	for i, w := range workflows {
		out[i] = lkp.diagnose(w.ID)
	}
	return out, nil
}

// Aggregate loads, filters and projects the target entity, then evaluates the
// aggregation tree — mirroring MemoryRepository.Aggregate but reading from disk.
func (r *SQLiteRepository) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	if len(q.Aggs) == 0 {
		return nil, nil
	}
	projected, err := r.snapshotProject(ctx, q.Entity, q.Filter, r.clock())
	if err != nil {
		return nil, err
	}
	return query.EvaluateAggs(projected, q.Aggs)
}

// snapshotProject loads the named entity's rows, then filters and projects each
// to map[string]any using the shared projection functions.
func (r *SQLiteRepository) snapshotProject(ctx context.Context, entity query.Entity, filter *query.Filter, now time.Time) ([]map[string]any, error) {
	switch entity {
	case query.EntityRepo:
		snap, err := r.loadRepos(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snap, filter, func(v model.Repo) map[string]any { return projectRepo(v) })

	case query.EntityCommit:
		snap, err := r.loadCommits(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snap, filter, func(v model.Commit) map[string]any { return projectCommit(v) })

	case query.EntityPullRequest:
		snap, err := r.loadPullRequests(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snap, filter, func(v model.PullRequest) map[string]any { return projectPullRequest(v, now) })

	case query.EntityWorkflow:
		snap, err := r.loadWorkflows(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snap, filter, func(v model.Workflow) map[string]any { return projectWorkflow(v) })

	case query.EntityWorkflowRun:
		snap, err := r.loadWorkflowRuns(ctx)
		if err != nil {
			return nil, err
		}
		lkp, err := r.ownerLookupFromDB(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snap, filter, func(v model.WorkflowRun) map[string]any { return projectWorkflowRun(v, lkp) })

	case query.EntityWorkflowTask:
		snapTasks, err := r.loadWorkflowTasks(ctx)
		if err != nil {
			return nil, err
		}
		runs, err := r.loadWorkflowRuns(ctx)
		if err != nil {
			return nil, err
		}
		snapRuns := make(map[string]model.WorkflowRun, len(runs))
		for _, run := range runs {
			snapRuns[run.ID] = run
		}
		lkp, err := r.ownerLookupFromDB(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snapTasks, filter, func(v model.WorkflowTask) map[string]any { return projectWorkflowTask(v, snapRuns, lkp) })

	case query.EntityTeam:
		snap, err := r.loadTeams(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, ensureUnknownTeam(snap), filter, func(v model.Team) map[string]any { return projectTeam(v) })

	case query.EntityComponent:
		snap, err := r.loadComponents(ctx)
		if err != nil {
			return nil, err
		}
		return filterProject(ctx, snap, filter, func(v model.Component) map[string]any { return projectComponent(v) })

	default:
		return nil, fmt.Errorf("unknown entity: %q", entity)
	}
}
