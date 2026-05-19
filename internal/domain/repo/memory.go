package repo

import (
	"context"
	"sync"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

type MemoryRepository struct {
	mu           sync.RWMutex
	repos        map[string]model.Repo
	commits      map[string]model.Commit
	pullRequests map[string]model.PullRequest
}

func NewMemory() *MemoryRepository {
	return &MemoryRepository{
		repos:        make(map[string]model.Repo),
		commits:      make(map[string]model.Commit),
		pullRequests: make(map[string]model.PullRequest),
	}
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
