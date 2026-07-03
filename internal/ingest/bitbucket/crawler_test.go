package bitbucket

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
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

// fakeBitbucketClient returns caller-supplied fixture data and records the
// sinceSHA argument passed to GetCommits so tests can assert cursor wiring.
// Set errOnSince to simulate a GetCommits failure when a cursor is active.
type fakeBitbucketClient struct {
	repos        map[string][]apiRepo
	commits      map[string][]apiCommit
	pullRequests map[string][]apiPullRequest
	rawFiles     map[string][]byte // key: "project/repo/path"
	rawErr       error             // returned by GetRawFile when set
	errOnSince   error             // returned by GetCommits when sinceSHA != ""

	mu        sync.Mutex
	sinceArgs map[string]string // key: "project/repo", value: last sinceSHA arg
	rawCalls  int               // number of GetRawFile calls
}

func (f *fakeBitbucketClient) GetRepos(_ context.Context, projectKey string) ([]apiRepo, error) {
	return f.repos[projectKey], nil
}

func (f *fakeBitbucketClient) GetCommits(_ context.Context, projectKey, repoSlug, sinceSHA string) ([]apiCommit, error) {
	f.mu.Lock()
	if f.sinceArgs == nil {
		f.sinceArgs = map[string]string{}
	}
	f.sinceArgs[projectKey+"/"+repoSlug] = sinceSHA
	f.mu.Unlock()

	if f.errOnSince != nil && sinceSHA != "" {
		return nil, f.errOnSince
	}
	return f.commits[projectKey+"/"+repoSlug], nil
}

func (f *fakeBitbucketClient) GetPullRequests(_ context.Context, projectKey, repoSlug string) ([]apiPullRequest, error) {
	return f.pullRequests[projectKey+"/"+repoSlug], nil
}

func (f *fakeBitbucketClient) GetRawFile(_ context.Context, projectKey, repoSlug, path string) ([]byte, bool, error) {
	f.mu.Lock()
	f.rawCalls++
	f.mu.Unlock()
	if f.rawErr != nil {
		return nil, false, f.rawErr
	}
	content, ok := f.rawFiles[projectKey+"/"+repoSlug+"/"+path]
	if !ok {
		return nil, false, nil
	}
	return content, true, nil
}

// fakeCursors is a map-backed Cursors for unit tests. It records every Save
// and Clear call so tests can assert correct cursor wiring.
type fakeCursors struct {
	mu     sync.Mutex
	data   map[string]string
	saves  []struct{ repoID, sha string }
	clears []string
}

func newFakeCursors() *fakeCursors {
	return &fakeCursors{data: map[string]string{}}
}

func (f *fakeCursors) Load(_ context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out, nil
}

func (f *fakeCursors) Save(_ context.Context, repoID, sha string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[repoID] = sha
	f.saves = append(f.saves, struct{ repoID, sha string }{repoID, sha})
	return nil
}

func (f *fakeCursors) Clear(_ context.Context, repoID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, repoID)
	f.clears = append(f.clears, repoID)
	return nil
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newCrawler(client BitbucketClient, pub *fakePublisher, projects []string) *Crawler {
	return NewCrawler(client, pub, newFakeCursors(), projects, newTestLogger())
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

// ---- Cursor wiring ----------------------------------------------------------

// TestCrawl_CommitCursorAdvances verifies that after the first Crawl the
// cursor is saved with the newest commit SHA, and that the second Crawl passes
// that SHA as the since argument to GetCommits.
func TestCrawl_CommitCursorAdvances(t *testing.T) {
	client := &fakeBitbucketClient{
		repos: map[string][]apiRepo{
			"PROJ": {{Slug: "svc", Name: "svc"}},
		},
		// Bitbucket returns commits newest-first.
		commits:      map[string][]apiCommit{"PROJ/svc": {{ID: "newer-sha"}, {ID: "older-sha"}}},
		pullRequests: map[string][]apiPullRequest{"PROJ/svc": {}},
	}
	store := newFakeCursors()
	pub := &fakePublisher{}
	crawler := NewCrawler(client, pub, store, []string{"PROJ"}, newTestLogger())

	// First crawl: no prior cursor — sinceSHA must be empty.
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := client.sinceArgs["PROJ/svc"]; got != "" {
		t.Errorf("first crawl: sinceSHA = %q, want empty", got)
	}
	if len(pub.commits) != 2 {
		t.Fatalf("first crawl: want 2 commits published, got %d", len(pub.commits))
	}
	if len(store.saves) != 1 || store.saves[0].sha != "newer-sha" {
		t.Errorf("first crawl: want cursor saved as newer-sha, got saves=%v", store.saves)
	}

	// Second crawl: cursor must be forwarded as sinceSHA.
	pub.commits = nil
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := client.sinceArgs["PROJ/svc"]; got != "newer-sha" {
		t.Errorf("second crawl: sinceSHA = %q, want newer-sha", got)
	}
}

// TestCrawl_CursorHydratedFromStore verifies that a cursor pre-existing in
// the store is loaded and used on the first Crawl, simulating a process
// restart with a persisted cursor.
func TestCrawl_CursorHydratedFromStore(t *testing.T) {
	client := &fakeBitbucketClient{
		repos:        map[string][]apiRepo{"PROJ": {{Slug: "svc", Name: "svc"}}},
		commits:      map[string][]apiCommit{"PROJ/svc": {}},
		pullRequests: map[string][]apiPullRequest{"PROJ/svc": {}},
	}
	store := newFakeCursors()
	store.data["PROJ/svc"] = "persisted-sha" // pre-load as if restored from KV

	pub := &fakePublisher{}
	crawler := NewCrawler(client, pub, store, []string{"PROJ"}, newTestLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := client.sinceArgs["PROJ/svc"]; got != "persisted-sha" {
		t.Errorf("sinceSHA = %q, want persisted-sha", got)
	}
}

// TestCrawl_CursorClearedOnGetCommitsError verifies that when GetCommits
// returns an error while a cursor is active (simulating a force-push), the
// cursor is cleared and a full refetch is attempted.
func TestCrawl_CursorClearedOnGetCommitsError(t *testing.T) {
	client := &fakeBitbucketClient{
		repos:        map[string][]apiRepo{"PROJ": {{Slug: "svc", Name: "svc"}}},
		commits:      map[string][]apiCommit{"PROJ/svc": {{ID: "fresh-sha"}}},
		pullRequests: map[string][]apiPullRequest{"PROJ/svc": {}},
		errOnSince:   errors.New("404 commit not found"),
	}
	store := newFakeCursors()
	store.data["PROJ/svc"] = "stale-sha" // simulate a force-pushed-away cursor

	pub := &fakePublisher{}
	crawler := NewCrawler(client, pub, store, []string{"PROJ"}, newTestLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Cursor cleared from store.
	if len(store.clears) != 1 || store.clears[0] != "PROJ/svc" {
		t.Errorf("want cursor cleared for PROJ/svc, got clears=%v", store.clears)
	}
	// Full refetch succeeded: commits published.
	if len(pub.commits) != 1 || pub.commits[0].SHA != "fresh-sha" {
		t.Errorf("want 1 commit published after fallback, got commits=%v", pub.commits)
	}
}

// ---- Self-onboarding (ADR-007) ---------------------------------------------

// fakeOnboardingPublisher records RepoOnboarding events published by the
// crawler and counts calls to distinguish "disabled" from "no config found".
type fakeOnboardingPublisher struct {
	onboardings []model.RepoOnboarding
}

func (f *fakeOnboardingPublisher) PublishRepoOnboarding(_ context.Context, o model.RepoOnboarding) error {
	f.onboardings = append(f.onboardings, o)
	return nil
}

func onboardingClient() *fakeBitbucketClient {
	return &fakeBitbucketClient{
		repos: map[string][]apiRepo{
			"PROJ": {{Slug: "onboarded", Name: "Onboarded"}, {Slug: "plain", Name: "Plain"}},
		},
		commits: map[string][]apiCommit{
			"PROJ/onboarded": {}, "PROJ/plain": {},
		},
		pullRequests: map[string][]apiPullRequest{
			"PROJ/onboarded": {}, "PROJ/plain": {},
		},
	}
}

func TestCrawl_SelfOnboard_PublishesEvent(t *testing.T) {
	client := onboardingClient()
	client.rawFiles = map[string][]byte{
		"PROJ/onboarded/.dalikamata.yaml": []byte("version: \"1\"\nteam: platform\ncomponent: backend\n"),
	}
	onboard := &fakeOnboardingPublisher{}
	crawler := NewCrawler(client, &fakePublisher{}, newFakeCursors(), []string{"PROJ"}, newTestLogger(),
		WithComponentConfig(onboard, []string{".dalikamata.yaml"}))

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Only the repo carrying a config is onboarded; the 404 for "plain" is skipped.
	if len(onboard.onboardings) != 1 {
		t.Fatalf("onboardings = %d, want 1 (%v)", len(onboard.onboardings), onboard.onboardings)
	}
	got := onboard.onboardings[0]
	if got.RepoID != model.NewRepoID("PROJ", "onboarded") {
		t.Errorf("RepoID = %q, want %q", got.RepoID, model.NewRepoID("PROJ", "onboarded"))
	}
	if got.Component != "backend" || got.Team != "platform" {
		t.Errorf("got component=%q team=%q, want backend/platform", got.Component, got.Team)
	}
}

func TestCrawl_SelfOnboard_FirstMatchingCandidateWins(t *testing.T) {
	client := onboardingClient()
	// Repo carries only the .yml variant; the earlier .yaml candidate 404s.
	client.rawFiles = map[string][]byte{
		"PROJ/onboarded/.dalikamata.yml": []byte("version: \"1\"\nteam: web\ncomponent: frontend\n"),
	}
	onboard := &fakeOnboardingPublisher{}
	crawler := NewCrawler(client, &fakePublisher{}, newFakeCursors(), []string{"PROJ"}, newTestLogger(),
		WithComponentConfig(onboard, []string{".dalikamata.yaml", ".dalikamata.yml"}))

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(onboard.onboardings) != 1 {
		t.Fatalf("onboardings = %d, want 1 (%v)", len(onboard.onboardings), onboard.onboardings)
	}
	got := onboard.onboardings[0]
	if got.Component != "frontend" || got.Team != "web" {
		t.Errorf("got component=%q team=%q, want frontend/web", got.Component, got.Team)
	}
}

func TestCrawl_SelfOnboard_InvalidConfigSkippedAndCrawlContinues(t *testing.T) {
	client := onboardingClient()
	client.rawFiles = map[string][]byte{
		"PROJ/onboarded/.dalikamata.yaml": []byte("version: \"1\"\nteam: platform\n"), // missing component
	}
	onboard := &fakeOnboardingPublisher{}
	pub := &fakePublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"PROJ"}, newTestLogger(),
		WithComponentConfig(onboard, []string{".dalikamata.yaml"}))

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Invalid config: no onboarding event, but repo ingestion still happened.
	if len(onboard.onboardings) != 0 {
		t.Errorf("onboardings = %d, want 0", len(onboard.onboardings))
	}
	if len(pub.repos) != 2 {
		t.Errorf("repos = %d, want 2 (crawl must continue)", len(pub.repos))
	}
}

func TestCrawl_SelfOnboard_FetchErrorSkippedAndCrawlContinues(t *testing.T) {
	client := onboardingClient()
	client.rawErr = errors.New("boom")
	onboard := &fakeOnboardingPublisher{}
	pub := &fakePublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"PROJ"}, newTestLogger(),
		WithComponentConfig(onboard, []string{".dalikamata.yaml"}))

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(onboard.onboardings) != 0 {
		t.Errorf("onboardings = %d, want 0", len(onboard.onboardings))
	}
	if len(pub.repos) != 2 {
		t.Errorf("repos = %d, want 2 (crawl must continue)", len(pub.repos))
	}
}

func TestCrawl_SelfOnboard_DisabledNeverFetches(t *testing.T) {
	client := onboardingClient()
	pub := &fakePublisher{}
	crawler := newCrawler(client, pub, []string{"PROJ"})

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.repos) != 2 {
		t.Errorf("repos = %d, want 2", len(pub.repos))
	}
	if client.rawCalls != 0 {
		t.Errorf("GetRawFile called %d times, want 0 when self-onboarding disabled", client.rawCalls)
	}
}
