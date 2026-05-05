package model

import (
	"time"
)

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
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
