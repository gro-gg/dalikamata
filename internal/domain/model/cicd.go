package model

import "time"

const (
	BuildStatusSuccess  = "SUCCESS"
	BuildStatusFailure  = "FAILURE"
	BuildStatusAborted  = "ABORTED"
	BuildStatusUnstable = "UNSTABLE"
)

type Workflow struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	RepoIDs []string `json:"repo_ids"`
}

type WorkflowRun struct {
	ID         string    `json:"id"`
	WorkflowID string    `json:"workflow_id"`
	Number     int       `json:"number"`
	Status     string    `json:"status"`
	Branch     string    `json:"branch"`
	CommitSHA  string    `json:"commit_sha"`
	StartedAt  time.Time `json:"started_at"`
	Duration   float64   `json:"duration"`

	// Enriched at query time by joining against the component/team index.
	// Empty (and omitted) when stored or published over the ingest stream.
	WorkflowName  string `json:"workflow_name,omitempty"`
	ComponentName string `json:"component_name,omitempty"`
	TeamName      string `json:"team_name,omitempty"`
}

type WorkflowTask struct {
	WorkflowRunID string    `json:"workflow_run_id"`
	Order         int       `json:"order"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	Duration      float64   `json:"duration"`

	// Enriched at query time by joining via the parent WorkflowRun.
	// Empty (and omitted) when stored or published over the ingest stream.
	WorkflowID    string `json:"workflow_id,omitempty"`
	WorkflowName  string `json:"workflow_name,omitempty"`
	ComponentName string `json:"component_name,omitempty"`
	TeamName      string `json:"team_name,omitempty"`
	Branch        string `json:"branch,omitempty"`
}
