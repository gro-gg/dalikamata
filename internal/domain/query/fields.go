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
	PRCreatedAt   = "createdAt"
	PRUpdatedAt   = "updatedAt"
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
	WorkflowID   = "id"
	WorkflowName = "name"
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
)

// WorkflowTask fields.
const (
	TaskWorkflowRunID = "workflow_run_id"
	TaskOrder         = "order"
	TaskName          = "name"
	TaskStatus        = "status"
	TaskStartedAt     = "started_at"
	TaskDuration      = "duration"
)
