package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	dalapi "codeberg.org/aeforged/dalikamata/api"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const (
	DefaultAPIAddr      = "0.0.0.0:2113"
	DefaultQueryTimeout = 30 * time.Second
)

// QueryFetcher is the outbound port the Server depends on to retrieve entity
// data and aggregations from the domain. *dalinats.QueryClient satisfies this
// interface without modification.
type QueryFetcher interface {
	QueryReposAll(ctx context.Context, q query.Query) ([]model.Repo, error)
	QueryCommitsAll(ctx context.Context, q query.Query) ([]model.Commit, error)
	QueryPullRequestsAll(ctx context.Context, q query.Query) ([]model.PullRequest, error)
	QueryWorkflowsAll(ctx context.Context, q query.Query) ([]model.Workflow, error)
	QueryWorkflowRunsAll(ctx context.Context, q query.Query) ([]model.WorkflowRun, error)
	QueryWorkflowTasksAll(ctx context.Context, q query.Query) ([]model.WorkflowTask, error)
	QueryTeamsAll(ctx context.Context, q query.Query) ([]model.Team, error)
	QueryComponentsAll(ctx context.Context, q query.Query) ([]model.Component, error)
	Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error)
}

// Option configures a Server.
type Option func(*Server)

// WithQueryTimeout overrides the per-request query deadline (default 30s).
func WithQueryTimeout(d time.Duration) Option {
	return func(s *Server) { s.queryTimeout = d }
}

// Server is an HTTP adapter that exposes the domain query layer as a JSON API.
type Server struct {
	client       QueryFetcher
	logger       *slog.Logger
	addr         string
	queryTimeout time.Duration
}

// NewServer creates a Server using client as its query backend.
func NewServer(client QueryFetcher, logger *slog.Logger, addr string, opts ...Option) *Server {
	s := &Server{
		client:       client,
		logger:       logger,
		addr:         DefaultAPIAddr,
		queryTimeout: DefaultQueryTimeout,
	}
	if addr != "" {
		s.addr = addr
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.newMux(),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.logger.Info("starting API HTTP server", "addr", s.addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("API HTTP server error", "error", err)
		}
	}()

	<-ctx.Done()

	if err := srv.Shutdown(context.Background()); err != nil {
		s.logger.Error("shutting down API HTTP server", "error", err)
	}
	wg.Wait()
	return nil
}

// newMux registers all entity endpoints and returns the mux.
func (s *Server) newMux() *http.ServeMux {
	type entry struct {
		path   string
		entity query.Entity
		fetch  func(context.Context, query.Query) (any, error)
	}

	entries := []entry{
		{"repos", query.EntityRepo, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryReposAll(ctx, q)
			if r == nil {
				r = []model.Repo{}
			}
			return r, err
		}},
		{"commits", query.EntityCommit, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryCommitsAll(ctx, q)
			if r == nil {
				r = []model.Commit{}
			}
			return r, err
		}},
		{"pullrequests", query.EntityPullRequest, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryPullRequestsAll(ctx, q)
			if r == nil {
				r = []model.PullRequest{}
			}
			return r, err
		}},
		{"workflows", query.EntityWorkflow, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryWorkflowsAll(ctx, q)
			if r == nil {
				r = []model.Workflow{}
			}
			return r, err
		}},
		{"workflowRuns", query.EntityWorkflowRun, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryWorkflowRunsAll(ctx, q)
			if r == nil {
				r = []model.WorkflowRun{}
			}
			return r, err
		}},
		{"workflowTasks", query.EntityWorkflowTask, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryWorkflowTasksAll(ctx, q)
			if r == nil {
				r = []model.WorkflowTask{}
			}
			return r, err
		}},
		{"teams", query.EntityTeam, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryTeamsAll(ctx, q)
			if r == nil {
				r = []model.Team{}
			}
			return r, err
		}},
		{"components", query.EntityComponent, func(ctx context.Context, q query.Query) (any, error) {
			r, err := s.client.QueryComponentsAll(ctx, q)
			if r == nil {
				r = []model.Component{}
			}
			return r, err
		}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(dalapi.Spec)
	})
	mux.HandleFunc("/api/v1/scalar.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(dalapi.ScalarJS)
	})
	mux.HandleFunc("/api/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(scalarDocsHTML))
	})
	for _, e := range entries {
		mux.HandleFunc("/api/v1/"+e.path, s.entityHandler(e.entity, e.fetch))
	}
	return mux
}

// hitsResponse is the JSON envelope for entity query results.
type hitsResponse struct {
	Entity string `json:"entity"`
	Size   int    `json:"size"`
	From   int    `json:"from"`
	Hits   any    `json:"hits"`
}

// aggregationResponse is the JSON envelope for aggregation-only results.
type aggregationResponse struct {
	Entity       string                             `json:"entity"`
	Aggregations map[string]query.AggregationResult `json:"aggregations"`
}

// entityHandler returns an http.HandlerFunc that dispatches GET (URL params)
// and POST (full query.Query body) to the domain layer, writing JSON back.
func (s *Server) entityHandler(entity query.Entity, fetchAll func(context.Context, query.Query) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			q   query.Query
			err error
		)

		switch r.Method {
		case http.MethodGet:
			q, err = parseQueryParams(r.URL.Query(), entity)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case http.MethodPost:
			if err = json.NewDecoder(r.Body).Decode(&q); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %s", err))
				return
			}
			q.Entity = entity
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), s.queryTimeout)
		defer cancel()

		if q.Size == -1 {
			aggs, err := s.client.Aggregate(ctx, q)
			if err != nil {
				s.handleQueryError(w, err)
				return
			}
			writeJSON(w, aggregationResponse{Entity: string(entity), Aggregations: aggs})
			return
		}

		hits, err := fetchAll(ctx, q)
		if err != nil {
			s.handleQueryError(w, err)
			return
		}
		writeJSON(w, hitsResponse{Entity: string(entity), Size: q.Size, From: q.From, Hits: hits})
	}
}

func (s *Server) handleQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "query timed out")
		return
	}
	s.logger.Error("query failed", "error", err)
	writeError(w, http.StatusInternalServerError, "query failed")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
}

const scalarDocsHTML = `<!doctype html>
<html lang="en">
  <head>
    <title>Dalikamata API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/api/v1/openapi.yaml"
    ></script>
    <script src="/api/v1/scalar.js"></script>
  </body>
</html>`
