package model

import "time"

const (
	BuildStatusSuccess  = "SUCCESS"
	BuildStatusFailure  = "FAILURE"
	BuildStatusAborted  = "ABORTED"
	BuildStatusUnstable = "UNSTABLE"
)

type Job struct {
	JobID string `json:"job_id"`
	Name  string `json:"name"`
}

type Build struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Number    int       `json:"number"`
	Status    string    `json:"status"`
	Branch    string    `json:"branch"`
	CommitSHA string    `json:"commit_sha"`
	StartedAt time.Time `json:"started_at"`
	Duration  float64   `json:"duration"`
}

type PipelineStage struct {
	BuildID   string    `json:"build_id"`
	Order     int       `json:"order"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Duration  float64   `json:"duration"`
}
