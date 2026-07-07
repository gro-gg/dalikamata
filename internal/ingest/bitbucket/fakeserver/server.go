package fakeserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// pagedResponse mirrors the Bitbucket Server pagination envelope.
type pagedResponse[T any] struct {
	Values        []T  `json:"values"`
	IsLastPage    bool `json:"isLastPage"`
	NextPageStart int  `json:"nextPageStart,omitempty"`
	Start         int  `json:"start"`
	Limit         int  `json:"limit"`
	Size          int  `json:"size"`
}

type apiProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type apiRepo struct {
	Slug    string     `json:"slug"`
	Name    string     `json:"name"`
	Project apiProject `json:"project"`
}

type apiGitUser struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
}

type apiCommit struct {
	ID                 string     `json:"id"`
	Message            string     `json:"message"`
	Author             apiGitUser `json:"author"`
	AuthorTimestamp    int64      `json:"authorTimestamp"`
	Committer          apiGitUser `json:"committer"`
	CommitterTimestamp int64      `json:"committerTimestamp"`
}

type apiPRUser struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

type apiPRParticipant struct {
	User   apiPRUser `json:"user"`
	Role   string    `json:"role"`
	Status string    `json:"status"`
}

type apiPRRef struct {
	DisplayId string `json:"displayId"`
}

type apiPullRequest struct {
	ID          int64              `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	State       string             `json:"state"`
	Author      apiPRParticipant   `json:"author"`
	Reviewers   []apiPRParticipant `json:"reviewers"`
	FromRef     apiPRRef           `json:"fromRef"`
	ToRef       apiPRRef           `json:"toRef"`
	CreatedDate int64              `json:"createdDate"`
	UpdatedDate int64              `json:"updatedDate"`
}

// ms converts a time.Time to Unix milliseconds.
func ms(t time.Time) int64 { return t.UnixMilli() }

var now = time.Now()

// fakeData holds all static fake data keyed by project key and repo slug.
var fakeProjects = map[string]apiProject{
	"PROJ":  {Key: "PROJ", Name: "Project Alpha"},
	"INFRA": {Key: "INFRA", Name: "Infrastructure"},
}

var fakeRepos = map[string][]apiRepo{
	"PROJ": {
		{Slug: "backend-api", Name: "Backend API", Project: fakeProjects["PROJ"]},
		{Slug: "frontend-app", Name: "Frontend App", Project: fakeProjects["PROJ"]},
		{Slug: "shared-lib", Name: "Shared Library", Project: fakeProjects["PROJ"]},
	},
	"INFRA": {
		{Slug: "k8s-configs", Name: "Kubernetes Configs", Project: fakeProjects["INFRA"]},
		{Slug: "terraform-modules", Name: "Terraform Modules", Project: fakeProjects["INFRA"]},
	},
}

var alice = apiGitUser{Name: "Alice Smith", EmailAddress: "alice@example.com"}
var bob = apiGitUser{Name: "Bob Jones", EmailAddress: "bob@example.com"}
var carol = apiGitUser{Name: "Carol White", EmailAddress: "carol@example.com"}

var alicePR = apiPRUser{Name: "asmith", EmailAddress: "alice@example.com", DisplayName: "Alice Smith"}
var bobPR = apiPRUser{Name: "bjones", EmailAddress: "bob@example.com", DisplayName: "Bob Jones"}
var carolPR = apiPRUser{Name: "cwhite", EmailAddress: "carol@example.com", DisplayName: "Carol White"}

// initialCommits holds the factory fixtures (newest-first per repo).
var initialCommits = map[string][]apiCommit{
	"backend-api": {
		{ID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", Message: "feat: add user authentication endpoint", Author: alice, AuthorTimestamp: ms(now.Add(-72 * time.Hour)), Committer: alice, CommitterTimestamp: ms(now.Add(-72 * time.Hour))},
		{ID: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3", Message: "fix: resolve null pointer in payment service", Author: bob, AuthorTimestamp: ms(now.Add(-48 * time.Hour)), Committer: bob, CommitterTimestamp: ms(now.Add(-48 * time.Hour))},
		{ID: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4", Message: "chore: update dependencies to latest versions", Author: carol, AuthorTimestamp: ms(now.Add(-24 * time.Hour)), Committer: carol, CommitterTimestamp: ms(now.Add(-24 * time.Hour))},
		{ID: "d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5", Message: "feat: implement rate limiting middleware", Author: alice, AuthorTimestamp: ms(now.Add(-12 * time.Hour)), Committer: alice, CommitterTimestamp: ms(now.Add(-12 * time.Hour))},
		{ID: "e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6", Message: "docs: add OpenAPI specification", Author: bob, AuthorTimestamp: ms(now.Add(-2 * time.Hour)), Committer: bob, CommitterTimestamp: ms(now.Add(-2 * time.Hour))},
	},
	"frontend-app": {
		{ID: "f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1", Message: "feat: redesign dashboard layout", Author: carol, AuthorTimestamp: ms(now.Add(-96 * time.Hour)), Committer: carol, CommitterTimestamp: ms(now.Add(-96 * time.Hour))},
		{ID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6b1c2", Message: "fix: fix broken navigation on mobile", Author: alice, AuthorTimestamp: ms(now.Add(-36 * time.Hour)), Committer: alice, CommitterTimestamp: ms(now.Add(-36 * time.Hour))},
		{ID: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6c2d3e4", Message: "style: apply design system tokens", Author: bob, AuthorTimestamp: ms(now.Add(-6 * time.Hour)), Committer: bob, CommitterTimestamp: ms(now.Add(-6 * time.Hour))},
	},
	"shared-lib": {
		{ID: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6d3e4f5a1", Message: "feat: add retry helper with exponential backoff", Author: alice, AuthorTimestamp: ms(now.Add(-120 * time.Hour)), Committer: alice, CommitterTimestamp: ms(now.Add(-120 * time.Hour))},
		{ID: "d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6e4f5a1b2c3", Message: "fix: handle edge case in JSON parser", Author: carol, AuthorTimestamp: ms(now.Add(-18 * time.Hour)), Committer: carol, CommitterTimestamp: ms(now.Add(-18 * time.Hour))},
	},
	"k8s-configs": {
		{ID: "e5f6a1b2c3d4e5f6a1b2c3d4e5f6f5a1b2c3d4e5", Message: "feat: add HPA config for backend services", Author: bob, AuthorTimestamp: ms(now.Add(-60 * time.Hour)), Committer: bob, CommitterTimestamp: ms(now.Add(-60 * time.Hour))},
		{ID: "f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6c3", Message: "chore: bump image tags to v2.3.1", Author: carol, AuthorTimestamp: ms(now.Add(-10 * time.Hour)), Committer: carol, CommitterTimestamp: ms(now.Add(-10 * time.Hour))},
	},
	"terraform-modules": {
		{ID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6d4e5", Message: "feat: add RDS module with multi-AZ support", Author: alice, AuthorTimestamp: ms(now.Add(-144 * time.Hour)), Committer: alice, CommitterTimestamp: ms(now.Add(-144 * time.Hour))},
		{ID: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6e5f6a1", Message: "fix: correct IAM role trust policy", Author: bob, AuthorTimestamp: ms(now.Add(-30 * time.Hour)), Committer: bob, CommitterTimestamp: ms(now.Add(-30 * time.Hour))},
		{ID: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6f6a1b2c3", Message: "refactor: extract VPC into reusable module", Author: carol, AuthorTimestamp: ms(now.Add(-4 * time.Hour)), Committer: carol, CommitterTimestamp: ms(now.Add(-4 * time.Hour))},
	},
}

// fakeRawFiles maps "{repoSlug}/{path}" to raw file content. Two repos ship a
// self-onboarding config (ADR-007); every other raw request returns 404.
var fakeRawFiles = map[string]string{
	"backend-api/.dalikamata.yaml": `version: "1"
team: platform
component: backend
`,
	"frontend-app/.dalikamata.yaml": `version: "1"
team: web
component: frontend
`,
}

var fakePullRequests = map[string][]apiPullRequest{
	"backend-api": {
		{
			ID: 1, Title: "feat: add user authentication endpoint", Description: "Implements JWT-based authentication for all API endpoints.",
			State:       "MERGED",
			Author:      apiPRParticipant{User: alicePR, Role: "AUTHOR", Status: "APPROVED"},
			Reviewers:   []apiPRParticipant{{User: bobPR, Role: "REVIEWER", Status: "APPROVED"}},
			FromRef:     apiPRRef{DisplayId: "feature/auth-endpoint"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-96 * time.Hour)), UpdatedDate: ms(now.Add(-72 * time.Hour)),
		},
		{
			ID: 2, Title: "fix: resolve null pointer in payment service", Description: "Fixes NPE when payment amount is missing from request body.",
			State:       "MERGED",
			Author:      apiPRParticipant{User: bobPR, Role: "AUTHOR", Status: "APPROVED"},
			Reviewers:   []apiPRParticipant{{User: carolPR, Role: "REVIEWER", Status: "APPROVED"}, {User: alicePR, Role: "REVIEWER", Status: "APPROVED"}},
			FromRef:     apiPRRef{DisplayId: "fix/payment-npe"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-60 * time.Hour)), UpdatedDate: ms(now.Add(-48 * time.Hour)),
		},
		{
			ID: 3, Title: "feat: implement rate limiting middleware", Description: "Adds token-bucket rate limiting per client IP.",
			State:       "OPEN",
			Author:      apiPRParticipant{User: alicePR, Role: "AUTHOR", Status: "UNAPPROVED"},
			Reviewers:   []apiPRParticipant{{User: bobPR, Role: "REVIEWER", Status: "UNAPPROVED"}},
			FromRef:     apiPRRef{DisplayId: "feature/rate-limiting"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-12 * time.Hour)), UpdatedDate: ms(now.Add(-12 * time.Hour)),
		},
	},
	"frontend-app": {
		{
			ID: 1, Title: "feat: redesign dashboard layout", Description: "Full redesign of the main dashboard using the new design system.",
			State:       "MERGED",
			Author:      apiPRParticipant{User: carolPR, Role: "AUTHOR", Status: "APPROVED"},
			Reviewers:   []apiPRParticipant{{User: alicePR, Role: "REVIEWER", Status: "APPROVED"}},
			FromRef:     apiPRRef{DisplayId: "feature/dashboard-redesign"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-120 * time.Hour)), UpdatedDate: ms(now.Add(-96 * time.Hour)),
		},
		{
			ID: 2, Title: "fix: fix broken navigation on mobile", Description: "Navigation bar collapses incorrectly on viewports < 768px.",
			State:       "OPEN",
			Author:      apiPRParticipant{User: alicePR, Role: "AUTHOR", Status: "UNAPPROVED"},
			Reviewers:   []apiPRParticipant{{User: carolPR, Role: "REVIEWER", Status: "UNAPPROVED"}},
			FromRef:     apiPRRef{DisplayId: "fix/mobile-nav"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-36 * time.Hour)), UpdatedDate: ms(now.Add(-36 * time.Hour)),
		},
	},
	"shared-lib": {
		{
			ID: 1, Title: "feat: add retry helper with exponential backoff", Description: "Generic retry utility with configurable jitter and max attempts.",
			State:       "MERGED",
			Author:      apiPRParticipant{User: alicePR, Role: "AUTHOR", Status: "APPROVED"},
			Reviewers:   []apiPRParticipant{{User: bobPR, Role: "REVIEWER", Status: "APPROVED"}, {User: carolPR, Role: "REVIEWER", Status: "APPROVED"}},
			FromRef:     apiPRRef{DisplayId: "feature/retry-helper"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-144 * time.Hour)), UpdatedDate: ms(now.Add(-120 * time.Hour)),
		},
	},
	"k8s-configs": {
		{
			ID: 1, Title: "feat: add HPA config for backend services", Description: "Horizontal Pod Autoscaler targeting 70% CPU utilisation.",
			State:       "OPEN",
			Author:      apiPRParticipant{User: bobPR, Role: "AUTHOR", Status: "UNAPPROVED"},
			Reviewers:   []apiPRParticipant{{User: carolPR, Role: "REVIEWER", Status: "UNAPPROVED"}},
			FromRef:     apiPRRef{DisplayId: "feature/hpa-backend"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-60 * time.Hour)), UpdatedDate: ms(now.Add(-60 * time.Hour)),
		},
	},
	"terraform-modules": {
		{
			ID: 1, Title: "feat: add RDS module with multi-AZ support", Description: "Reusable RDS module with automated failover and parameter groups.",
			State:       "MERGED",
			Author:      apiPRParticipant{User: alicePR, Role: "AUTHOR", Status: "APPROVED"},
			Reviewers:   []apiPRParticipant{{User: bobPR, Role: "REVIEWER", Status: "APPROVED"}},
			FromRef:     apiPRRef{DisplayId: "feature/rds-module"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-168 * time.Hour)), UpdatedDate: ms(now.Add(-144 * time.Hour)),
		},
		{
			ID: 2, Title: "fix: correct IAM role trust policy", Description: "Trust policy was too permissive; restricts to specific AWS accounts.",
			State:       "DECLINED",
			Author:      apiPRParticipant{User: bobPR, Role: "AUTHOR", Status: "UNAPPROVED"},
			Reviewers:   []apiPRParticipant{{User: alicePR, Role: "REVIEWER", Status: "NEEDS_WORK"}},
			FromRef:     apiPRRef{DisplayId: "fix/iam-trust-policy"},
			ToRef:       apiPRRef{DisplayId: "main"},
			CreatedDate: ms(now.Add(-48 * time.Hour)), UpdatedDate: ms(now.Add(-30 * time.Hour)),
		},
	},
}

// Server is a fake Bitbucket Server HTTP server. Each Server instance owns a
// mutable copy of the commit fixtures so that AddCommit does not affect other
// instances (useful in parallel tests).
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	mu         sync.Mutex
	commits    map[string][]apiCommit // mutable per-instance, newest-first
}

// New creates a new fake Bitbucket Server listening on addr.
func New(addr string, logger *slog.Logger) *Server {
	commitsCopy := make(map[string][]apiCommit, len(initialCommits))
	for k, v := range initialCommits {
		c := make([]apiCommit, len(v))
		copy(c, v)
		commitsCopy[k] = c
	}

	mux := http.NewServeMux()
	s := &Server{
		httpServer: &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 30 * time.Second},
		logger:     logger,
		commits:    commitsCopy,
	}
	mux.HandleFunc("GET /rest/api/1.0/projects/{projectKey}/repos", s.handleRepos)
	mux.HandleFunc("GET /rest/api/1.0/projects/{projectKey}/repos/{repoSlug}/commits", s.handleCommits)
	mux.HandleFunc("GET /rest/api/1.0/projects/{projectKey}/repos/{repoSlug}/pull-requests", s.handlePullRequests)
	mux.HandleFunc("GET /rest/api/1.0/projects/{projectKey}/repos/{repoSlug}/raw/{path...}", s.handleRawFile)
	return s
}

// Start runs the HTTP server and blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("fake bitbucket server starting", "addr", s.httpServer.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("fake bitbucket server shutting down")
		return s.httpServer.Shutdown(context.Background())
	}
}

// AddCommit prepends c to repoSlug's commit list so it becomes the newest
// commit returned by subsequent GET /commits requests. Safe to call
// concurrently with in-flight HTTP requests.
func (s *Server) AddCommit(repoSlug string, c apiCommit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commits[repoSlug] = append([]apiCommit{c}, s.commits[repoSlug]...)
}

// NewCommit is a convenience helper that builds an apiCommit with the given
// id and a committer timestamp of now, suitable for passing to AddCommit.
func NewCommit(id, message string) apiCommit {
	return apiCommit{
		ID:                 id,
		Message:            message,
		Author:             alice,
		AuthorTimestamp:    ms(time.Now()),
		Committer:          alice,
		CommitterTimestamp: ms(time.Now()),
	}
}

func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	projectKey := r.PathValue("projectKey")
	s.logger.Info("fake: list repos", "project", projectKey)

	repos := fakeRepos[projectKey]
	writePagedJSON(w, repos)
}

func (s *Server) handleCommits(w http.ResponseWriter, r *http.Request) {
	projectKey := r.PathValue("projectKey")
	repoSlug := r.PathValue("repoSlug")
	since := r.URL.Query().Get("since")
	s.logger.Info("fake: list commits", "project", projectKey, "repo", repoSlug, "since", since)

	s.mu.Lock()
	all := make([]apiCommit, len(s.commits[repoSlug]))
	copy(all, s.commits[repoSlug])
	s.mu.Unlock()

	if since != "" {
		// Commits are stored newest-first. Return only commits that appear
		// before (i.e. are newer than) the since SHA.
		found := false
		for i, c := range all {
			if c.ID == since {
				all = all[:i]
				found = true
				break
			}
		}
		if !found {
			// The since SHA is unknown — simulate Bitbucket's 400/404 for a
			// rewritten commit so the crawler's force-push recovery triggers.
			http.Error(w, "since commit not found", http.StatusBadRequest)
			return
		}
	}

	writePagedJSON(w, all)
}

func (s *Server) handlePullRequests(w http.ResponseWriter, r *http.Request) {
	projectKey := r.PathValue("projectKey")
	repoSlug := r.PathValue("repoSlug")
	s.logger.Info("fake: list pull-requests", "project", projectKey, "repo", repoSlug)

	prs := fakePullRequests[repoSlug]
	writePagedJSON(w, prs)
}

func (s *Server) handleRawFile(w http.ResponseWriter, r *http.Request) {
	repoSlug := r.PathValue("repoSlug")
	path := r.PathValue("path")
	s.logger.Info("fake: raw file", "repo", repoSlug, "path", path)

	content, ok := fakeRawFiles[repoSlug+"/"+path]
	if !ok {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// writePagedJSON encodes a slice as a Bitbucket-style paged response with pagination
// applied from the request's start/limit query parameters.
func writePagedJSON[T any](w http.ResponseWriter, all []T) {
	// Pagination would normally come from the request, but for the fake we just
	// return everything in one page to keep the implementation simple.
	resp := pagedResponse[T]{
		Values:     all,
		IsLastPage: true,
		Start:      0,
		Limit:      len(all),
		Size:       len(all),
	}
	if all == nil {
		resp.Values = []T{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Addr returns the base URL of the server.
func (s *Server) Addr() string {
	return "http://" + s.httpServer.Addr
}
