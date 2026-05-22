package fakeserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	classWorkflowJob = "org.jenkinsci.plugins.workflow.job.WorkflowJob"
	classGitBuildData = "hudson.plugins.git.util.BuildData"
)

// These types mirror the jenkins package's API structs. The fakeserver is a
// separate package so it defines its own copies (same as the Bitbucket fakeserver).

type apiJobList struct {
	Jobs []apiJob `json:"jobs"`
}

type apiJob struct {
	Class string `json:"_class"`
	Name  string `json:"name"`
}

type apiBuildList struct {
	Builds []apiBuild `json:"builds"`
}

type apiBuild struct {
	Number     int              `json:"number"`
	Result     string           `json:"result"`
	Timestamp  int64            `json:"timestamp"`
	Duration   int64            `json:"duration"`
	InProgress bool             `json:"inProgress"`
	Actions    []apiBuildAction `json:"actions"`
}

type apiBuildAction struct {
	Class             string       `json:"_class"`
	LastBuiltRevision *apiRevision `json:"lastBuiltRevision,omitempty"`
}

type apiRevision struct {
	SHA1   string      `json:"SHA1"`
	Branch []apiBranch `json:"branch"`
}

type apiBranch struct {
	Name string `json:"name"`
}

type apiWFDescribe struct {
	Stages []apiStage `json:"stages"`
}

type apiStage struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	StartTimeMillis int64  `json:"startTimeMillis"`
	DurationMillis  int64  `json:"durationMillis"`
}

func ms(t time.Time) int64 { return t.UnixMilli() }

var now = time.Now()

// Pre-populated fixture data: 2 WorkflowJobs × 3 builds × 3 stages.

var rootJobs = []apiJob{
	{Class: classWorkflowJob, Name: "build-backend"},
	{Class: classWorkflowJob, Name: "build-frontend"},
}

var fakeBuilds = map[string][]apiBuild{
	"build-backend": {
		{
			Number: 1, Result: "SUCCESS", Timestamp: ms(now.Add(-72 * time.Hour)), Duration: 120_000, InProgress: false,
			Actions: []apiBuildAction{{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "aabb1100", Branch: []apiBranch{{Name: "origin/main"}}}}},
		},
		{
			Number: 2, Result: "FAILURE", Timestamp: ms(now.Add(-48 * time.Hour)), Duration: 45_000, InProgress: false,
			Actions: []apiBuildAction{{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "ccdd2200", Branch: []apiBranch{{Name: "origin/feature/auth"}}}}},
		},
		{
			Number: 3, Result: "SUCCESS", Timestamp: ms(now.Add(-24 * time.Hour)), Duration: 118_000, InProgress: false,
			Actions: []apiBuildAction{{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "eeff3300", Branch: []apiBranch{{Name: "origin/main"}}}}},
		},
	},
	"build-frontend": {
		{
			Number: 1, Result: "SUCCESS", Timestamp: ms(now.Add(-96 * time.Hour)), Duration: 90_000, InProgress: false,
			Actions: []apiBuildAction{{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "ff001122", Branch: []apiBranch{{Name: "origin/main"}}}}},
		},
		{
			Number: 2, Result: "SUCCESS", Timestamp: ms(now.Add(-36 * time.Hour)), Duration: 88_000, InProgress: false,
			Actions: []apiBuildAction{{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "33445566", Branch: []apiBranch{{Name: "origin/main"}}}}},
		},
		{
			Number: 3, Result: "FAILURE", Timestamp: ms(now.Add(-12 * time.Hour)), Duration: 30_000, InProgress: false,
			Actions: []apiBuildAction{{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "77889900", Branch: []apiBranch{{Name: "origin/fix/navbar"}}}}},
		},
	},
}

var fakeStages = []apiStage{
	{Name: "Checkout", Status: "SUCCESS", StartTimeMillis: ms(now), DurationMillis: 5_000},
	{Name: "Build", Status: "SUCCESS", StartTimeMillis: ms(now.Add(5 * time.Second)), DurationMillis: 45_000},
	{Name: "Test", Status: "SUCCESS", StartTimeMillis: ms(now.Add(50 * time.Second)), DurationMillis: 60_000},
}

// Server is a fake Jenkins HTTP server.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates a new fake Jenkins Server listening on addr.
func New(addr string, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{
		httpServer: &http.Server{Addr: addr, Handler: mux},
		logger:     logger,
	}
	mux.HandleFunc("GET /api/json", s.handleRootJobs)
	mux.HandleFunc("GET /job/{name}/api/json", s.handleJobAPI)
	mux.HandleFunc("GET /job/{name}/{buildnum}/wfapi/describe", s.handleStages)
	return s
}

// Start runs the HTTP server and blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("fake jenkins server starting", "addr", s.httpServer.Addr)
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
		s.logger.Info("fake jenkins server shutting down")
		return s.httpServer.Shutdown(context.Background())
	}
}

// Addr returns the base URL of the server.
func (s *Server) Addr() string {
	return "http://" + s.httpServer.Addr
}

func (s *Server) handleRootJobs(w http.ResponseWriter, _ *http.Request) {
	s.logger.Info("fake: list root jobs")
	writeJSON(w, apiJobList{Jobs: rootJobs})
}

func (s *Server) handleJobAPI(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tree := r.URL.Query().Get("tree")
	s.logger.Info("fake: job api", "name", name)

	if strings.HasPrefix(tree, "builds") {
		builds := fakeBuilds[name]
		writeJSON(w, apiBuildList{Builds: builds})
		return
	}
	// No nested jobs — every item is a top-level WorkflowJob
	writeJSON(w, apiJobList{Jobs: []apiJob{}})
}

func (s *Server) handleStages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	buildnum := r.PathValue("buildnum")
	s.logger.Info("fake: stages", "name", name, "build", buildnum)
	writeJSON(w, apiWFDescribe{Stages: fakeStages})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
