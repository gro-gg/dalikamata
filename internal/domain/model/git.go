package model

import (
	"path"
	"time"
)

const (
	PullRequestStateOpen     = "OPEN"
	PullRequestStateMerged   = "MERGED"
	PullRequestStateDeclined = "DECLINED"
)

func NewRepoID(projectKey, repoSlug string) string {
	return path.Join(projectKey, repoSlug)
}

func NewPullRequestID(projectKey, repoSlug, prNumber string) string {
	return path.Join(projectKey, repoSlug, prNumber)
}

type Repo struct {
	RepoID string `json:"repo_id"`
	Name   string `json:"name"`
}

type Commit struct {
	SHA       string    `json:"sha"`
	RepoID    string    `json:"repo_id"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
}

type PullRequest struct {
	ID          string    `json:"id"`
	RepoID      string    `json:"repo_id"`
	Name        string    `json:"name"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	Author      string    `json:"author"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
