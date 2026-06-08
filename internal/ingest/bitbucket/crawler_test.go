package bitbucket

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// fakePublisher records every event published by the Crawler.
type fakePublisher struct {
	repos        []model.Repo
	commits      []model.Commit
	pullRequests []model.PullRequest
}

func (f *fakePublisher) PublishRepo(_ context.Context, r model.Repo) error {
	f.repos = append(f.repos, r)
	return nil
}

func (f *fakePublisher) PublishCommit(_ context.Context, c model.Commit) error {
	f.commits = append(f.commits, c)
	return nil
}

func (f *fakePublisher) PublishPullRequest(_ context.Context, pr model.PullRequest) error {
	f.pullRequests = append(f.pullRequests, pr)
	return nil
}

// fakeBitbucketClient returns caller-supplied fixture data.
type fakeBitbucketClient struct {
	repos        map[string][]apiRepo
	commits      map[string][]apiCommit
	pullRequests map[string][]apiPullRequest
}

func (f *fakeBitbucketClient) GetRepos(_ context.Context, projectKey string) ([]apiRepo, error) {
	return f.repos[projectKey], nil
}

func (f *fakeBitbucketClient) GetCommits(_ context.Context, projectKey, repoSlug, _ string) ([]apiCommit, error) {
	return f.commits[projectKey+"/"+repoSlug], nil
}

func (f *fakeBitbucketClient) GetPullRequests(_ context.Context, projectKey, repoSlug string) ([]apiPullRequest, error) {
	return f.pullRequests[projectKey+"/"+repoSlug], nil
}

func newCrawler(client BitbucketClient, pub *fakePublisher, projects []string) *Crawler {
	return NewCrawler(client, pub, projects, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// ---- Repo mapping -----------------------------------------------------------

func TestCrawl_RepoIDAndNameMapped(t *testing.T) {
	client := &fakeBitbucketClient{
		repos: map[string][]apiRepo{
			"PROJ": {{Slug: "my-repo", Name: "My Repo"}},
		},
		commits:      map[string][]apiCommit{"PROJ/my-repo": {}},
		pullRequests: map[string][]apiPullRequest{"PROJ/my-repo": {}},
	}
	pub := &fakePublisher{}
	if err := newCrawler(client, pub, []string{"PROJ"}).Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.repos) != 1 {
		t.Fatalf("repos = %d, want 1", len(pub.repos))
	}
	if pub.repos[0].RepoID != model.NewRepoID("PROJ", "my-repo") {
		t.Errorf("RepoID = %q, want %q", pub.repos[0].RepoID, model.NewRepoID("PROJ", "my-repo"))
	}
	if pub.repos[0].Name != "My Repo" {
		t.Errorf("Name = %q, want %q", pub.repos[0].Name, "My Repo")
	}
}

// ---- Commit mapping ---------------------------------------------------------

func TestCrawl_CommitFieldsMapped(t *testing.T) {
	ts := int64(1_704_067_200_000) // 2024-01-01T00:00:00Z in milliseconds
	client := &fakeBitbucketClient{
		repos: map[string][]apiRepo{
			"PROJ": {{Slug: "svc", Name: "svc"}},
		},
		commits: map[string][]apiCommit{
			"PROJ/svc": {
				{ID: "abc123", Author: apiGitUser{Name: "alice"}, AuthorTimestamp: ts},
			},
		},
		pullRequests: map[string][]apiPullRequest{"PROJ/svc": {}},
	}
	pub := &fakePublisher{}
	if err := newCrawler(client, pub, []string{"PROJ"}).Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.commits) != 1 {
		t.Fatalf("commits = %d, want 1", len(pub.commits))
	}
	c := pub.commits[0]
	if c.SHA != "abc123" {
		t.Errorf("SHA = %q, want abc123", c.SHA)
	}
	if c.Author != "alice" {
		t.Errorf("Author = %q, want alice", c.Author)
	}
	wantRepoID := model.NewRepoID("PROJ", "svc")
	if c.RepoID != wantRepoID {
		t.Errorf("RepoID = %q, want %q", c.RepoID, wantRepoID)
	}
	wantTS := time.UnixMilli(ts)
	if !c.Timestamp.Equal(wantTS) {
		t.Errorf("Timestamp = %v, want %v", c.Timestamp, wantTS)
	}
}

// ---- Pull-request mapping ---------------------------------------------------

func TestCrawl_PullRequestFieldsMapped(t *testing.T) {
	created := int64(1_704_067_200_000) // 2024-01-01 00:00:00 UTC
	updated := int64(1_704_153_600_000) // 2024-01-02 00:00:00 UTC
	client := &fakeBitbucketClient{
		repos: map[string][]apiRepo{
			"PROJ": {{Slug: "svc", Name: "svc"}},
		},
		commits: map[string][]apiCommit{"PROJ/svc": {}},
		pullRequests: map[string][]apiPullRequest{
			"PROJ/svc": {
				{
					ID:          42,
					Title:       "Fix the thing",
					Description: "A fix",
					State:       model.PullRequestStateMerged,
					Author:      apiPRParticipant{User: apiPRUser{DisplayName: "Bob"}},
					CreatedDate: created,
					UpdatedDate: updated,
				},
			},
		},
	}
	pub := &fakePublisher{}
	if err := newCrawler(client, pub, []string{"PROJ"}).Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.pullRequests) != 1 {
		t.Fatalf("pull requests = %d, want 1", len(pub.pullRequests))
	}
	pr := pub.pullRequests[0]

	wantID := model.NewPullRequestID("PROJ", "svc", "42")
	if pr.ID != wantID {
		t.Errorf("ID = %q, want %q", pr.ID, wantID)
	}
	wantRepoID := model.NewRepoID("PROJ", "svc")
	if pr.RepoID != wantRepoID {
		t.Errorf("RepoID = %q, want %q", pr.RepoID, wantRepoID)
	}
	if pr.Title != "Fix the thing" {
		t.Errorf("Title = %q, want %q", pr.Title, "Fix the thing")
	}
	if pr.Description != "A fix" {
		t.Errorf("Description = %q, want %q", pr.Description, "A fix")
	}
	if pr.State != model.PullRequestStateMerged {
		t.Errorf("State = %q, want MERGED", pr.State)
	}
	if pr.Author != "Bob" {
		t.Errorf("Author = %q, want Bob", pr.Author)
	}
	if !pr.CreatedAt.Equal(time.UnixMilli(created)) {
		t.Errorf("CreatedAt = %v, want %v", pr.CreatedAt, time.UnixMilli(created))
	}
	if !pr.UpdatedAt.Equal(time.UnixMilli(updated)) {
		t.Errorf("UpdatedAt = %v, want %v", pr.UpdatedAt, time.UnixMilli(updated))
	}
}

// ---- Multi-project / multi-repo counts -------------------------------------

func TestCrawl_MultipleProjectsAndRepos(t *testing.T) {
	client := &fakeBitbucketClient{
		repos: map[string][]apiRepo{
			"ALPHA": {
				{Slug: "repo-a", Name: "Repo A"},
				{Slug: "repo-b", Name: "Repo B"},
			},
			"BETA": {
				{Slug: "repo-c", Name: "Repo C"},
			},
		},
		commits: map[string][]apiCommit{
			"ALPHA/repo-a": {{ID: "sha1"}},
			"ALPHA/repo-b": {{ID: "sha2"}, {ID: "sha3"}},
			"BETA/repo-c":  {},
		},
		pullRequests: map[string][]apiPullRequest{
			"ALPHA/repo-a": {{ID: 1, State: "OPEN", Author: apiPRParticipant{User: apiPRUser{DisplayName: "alice"}}}},
			"ALPHA/repo-b": {},
			"BETA/repo-c":  {},
		},
	}
	pub := &fakePublisher{}
	if err := newCrawler(client, pub, []string{"ALPHA", "BETA"}).Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.repos) != 3 {
		t.Errorf("repos = %d, want 3", len(pub.repos))
	}
	if len(pub.commits) != 3 {
		t.Errorf("commits = %d, want 3", len(pub.commits))
	}
	if len(pub.pullRequests) != 1 {
		t.Errorf("pull requests = %d, want 1", len(pub.pullRequests))
	}
}

// ---- Empty project ----------------------------------------------------------

func TestCrawl_EmptyProject(t *testing.T) {
	client := &fakeBitbucketClient{
		repos:        map[string][]apiRepo{"EMPTY": {}},
		commits:      map[string][]apiCommit{},
		pullRequests: map[string][]apiPullRequest{},
	}
	pub := &fakePublisher{}
	if err := newCrawler(client, pub, []string{"EMPTY"}).Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.repos) != 0 || len(pub.commits) != 0 || len(pub.pullRequests) != 0 {
		t.Errorf("expected nothing published; got repos=%d commits=%d prs=%d",
			len(pub.repos), len(pub.commits), len(pub.pullRequests))
	}
}
