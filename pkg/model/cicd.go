package model

import "time"

const (
	BuildStatusSuccess  = "SUCCESS"
	BuildStatusFailure  = "FAILURE"
	BuildStatusAborted  = "ABORTED"
	BuildStatusUnstable = "UNSTABLE"
)

type Workflow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
}

type WorkflowTask struct {
	ID        string    `json:"id"`
	Order     int       `json:"order"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Duration  float64   `json:"duration"`
}
