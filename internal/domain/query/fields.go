package query

// Field name constants for each domain entity. Values match the entity's JSON
// tags so they are stable across refactors.

// Repo fields.
const (
	RepoID   = "repo_id"
	RepoName = "name"
)

// Commit fields.
const (
	CommitSHA       = "sha"
	CommitRepoID    = "repo_id"
	CommitAuthor    = "author"
	CommitTimestamp = "timestamp"
)

// PullRequest fields.
const (
	PRRepoID      = "repo_id"
	PRAuthor      = "author"
	PRState       = "state"
	PRCreatedAt   = "created_at"
	PRUpdatedAt   = "updated_at"
	PRName        = "name"
	PRTitle       = "title"
	PRDescription = "description"
	PRID          = "id"

	// PRCycleTimeSeconds is a computed field: seconds from PR creation to its
	// final state (MERGED/DECLINED) or to the current time for OPEN PRs.
	// Materialized by projectPullRequest at read time — not stored on the model.
	PRCycleTimeSeconds = "cycle_time_seconds"
)

// Workflow fields.
const (
	WorkflowID     = "id"
	WorkflowName   = "name"
	WorkflowRepoID = "repo_id"
)

// WorkflowRun fields.
const (
	RunID         = "id"
	RunWorkflowID = "workflow_id"
	RunNumber     = "number"
	RunStatus     = "status"
	RunBranch     = "branch"
	RunCommitSHA  = "commit_sha"
	RunStartedAt  = "started_at"
	RunDuration   = "duration"

	// Enriched at projection time by joining against the component index:
	// workflow_id → component → team. RunWorkflowName is dereferenced from
	// the Workflow record. None of these are stored on model.WorkflowRun.
	RunWorkflowName  = "workflow_name"
	RunComponentName = "component_name"
	RunTeamName      = "team_name"
)

// WorkflowTask fields.
const (
	TaskWorkflowRunID = "workflow_run_id"
	TaskOrder         = "order"
	TaskName          = "name"
	TaskStatus        = "status"
	TaskStartedAt     = "started_at"
	TaskDuration      = "duration"

	// Enriched at projection time. Tasks only carry WorkflowRunID on the
	// model, so workflow_id/workflow_name/component_name/team_name/branch are
	// all looked up via the parent run.
	TaskWorkflowID    = "workflow_id"
	TaskWorkflowName  = "workflow_name"
	TaskComponentName = "component_name"
	TaskTeamName      = "team_name"
	TaskBranch        = "branch"
)

// Team fields.
const (
	TeamName = "name"
)

// Component fields.
const (
	ComponentName     = "name"
	ComponentTeamName = "team_name"
)
