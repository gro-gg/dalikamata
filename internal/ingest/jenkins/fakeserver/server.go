package fakeserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	classWorkflowJob  = "org.jenkinsci.plugins.workflow.job.WorkflowJob"
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

// stageSpec names a pipeline stage and its fraction of total build time.
type stageSpec struct {
	name   string
	weight float64
}

// jobConfig defines the stage template and per-build durations for one job.
// Durations (ms) are spread across the workflow_run_duration_seconds histogram
// buckets [60, 300, 900, 1800, 3600, 7200, 21600] so the Grafana dashboard
// shows a populated distribution rather than a single bar.
type jobConfig struct {
	stages    []stageSpec
	durations []int64 // one entry per build, in milliseconds
}

var jobConfigs = map[string]jobConfig{
	// build-backend: fast CI — mostly <5 min, some slower outliers.
	"build-backend": {
		stages: []stageSpec{{"Checkout", 0.05}, {"Build", 0.40}, {"Test", 0.45}, {"Lint", 0.10}},
		durations: []int64{
			25_000, 80_000, 140_000, 200_000, 280_000,
			370_000, 480_000, 610_000, 760_000, 950_000,
		},
	},
	// test-backend: medium — integration tests push into 5–30 min range.
	"test-backend": {
		stages: []stageSpec{{"Checkout", 0.05}, {"Unit Tests", 0.35}, {"Integration Tests", 0.60}},
		durations: []int64{
			100_000, 250_000, 420_000, 580_000, 730_000,
			950_000, 1_150_000, 1_400_000, 1_700_000, 2_100_000,
		},
	},
	// deploy-backend: slow — deploy + smoke tests run 5–60 min.
	"deploy-backend": {
		stages: []stageSpec{{"Checkout", 0.04}, {"Build", 0.28}, {"Deploy", 0.40}, {"Smoke Test", 0.28}},
		durations: []int64{
			310_000, 490_000, 680_000, 900_000, 1_120_000,
			1_380_000, 1_700_000, 2_100_000, 2_700_000, 3_500_000,
		},
	},
	// build-frontend: fast CI — comparable to backend but install adds overhead.
	"build-frontend": {
		stages: []stageSpec{{"Checkout", 0.05}, {"Install", 0.15}, {"Build", 0.35}, {"Test", 0.45}},
		durations: []int64{
			30_000, 65_000, 110_000, 170_000, 240_000,
			330_000, 440_000, 580_000, 730_000, 900_000,
		},
	},
	// deploy-frontend: slowest — E2E tests dominate, often 10–80 min.
	"deploy-frontend": {
		stages: []stageSpec{{"Checkout", 0.04}, {"Build", 0.18}, {"Deploy", 0.28}, {"E2E Tests", 0.50}},
		durations: []int64{
			420_000, 660_000, 930_000, 1_200_000, 1_500_000,
			1_850_000, 2_300_000, 2_900_000, 3_700_000, 4_800_000,
		},
	},
}

// jobOrder fixes the iteration order of jobs so the fixture is deterministic.
var jobOrder = []string{
	"build-backend", "test-backend", "deploy-backend",
	"build-frontend", "deploy-frontend",
}

// buildResults cycles through results: 7 SUCCESS, 2 FAILURE, 1 ABORTED per 10 builds.
var buildResults = [10]string{
	"SUCCESS", "SUCCESS", "FAILURE", "SUCCESS", "SUCCESS",
	"SUCCESS", "ABORTED", "SUCCESS", "FAILURE", "SUCCESS",
}

// buildBranches cycles through realistic branch names.
var buildBranches = [10]string{
	"origin/main", "origin/main", "origin/feature/auth",
	"origin/main", "origin/main",
	"origin/fix/timeout", "origin/main", "origin/main",
	"origin/feature/perf", "origin/main",
}

func ms(t time.Time) int64 { return t.UnixMilli() }

var rootJobs []apiJob
var fakeBuilds map[string][]apiBuild

// epoch is computed once so stage timestamps stay consistent within a server run.
var epoch = time.Now()

func init() {
	rootJobs = make([]apiJob, len(jobOrder))
	for i, name := range jobOrder {
		rootJobs[i] = apiJob{Class: classWorkflowJob, Name: name}
	}

	fakeBuilds = make(map[string][]apiBuild, len(jobOrder))

	// Spread builds over 14 days; the most recent build is ~1 hour ago.
	const span = 14*24*time.Hour - time.Hour

	for jobIdx, name := range jobOrder {
		cfg := jobConfigs[name]
		n := len(cfg.durations)
		step := span / time.Duration(n-1)
		builds := make([]apiBuild, n)
		for i := 0; i < n; i++ {
			// Rotate result and branch patterns by job so failures fall on
			// different build numbers for different jobs.
			result := buildResults[(i+jobIdx*3)%10]
			branch := buildBranches[(i+jobIdx*2)%10]
			buildTime := epoch.Add(-span + time.Duration(i)*step)
			builds[i] = apiBuild{
				Number:    i + 1,
				Result:    result,
				Timestamp: ms(buildTime),
				Duration:  cfg.durations[i],
				Actions: []apiBuildAction{{
					Class: classGitBuildData,
					LastBuiltRevision: &apiRevision{
						SHA1:   deterministicSHA(jobIdx, i),
						Branch: []apiBranch{{Name: branch}},
					},
				}},
			}
		}
		fakeBuilds[name] = builds
	}
}

// deterministicSHA returns a stable 8-char hex string for a given (job, build) pair.
func deterministicSHA(jobIdx, buildIdx int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, 8)
	v := jobIdx*97 + buildIdx*43 + 17
	for k := range b {
		v = (v*31 + k + 1) & 0x7fffffff
		b[k] = hexChars[v%16]
	}
	return string(b)
}

// stagesForBuild computes stage data for a given job and build number.
// Stage durations are proportional to the total build duration; the final
// stage of a FAILURE or ABORTED build carries the matching status.
func stagesForBuild(jobName string, buildNum int) []apiStage {
	cfg, ok := jobConfigs[jobName]
	if !ok {
		return nil
	}
	builds, ok := fakeBuilds[jobName]
	if !ok {
		return nil
	}
	var b *apiBuild
	for i := range builds {
		if builds[i].Number == buildNum {
			b = &builds[i]
			break
		}
	}
	if b == nil {
		return nil
	}
	stages := make([]apiStage, len(cfg.stages))
	var elapsed int64
	for i, spec := range cfg.stages {
		dur := int64(float64(b.Duration) * spec.weight)
		status := "SUCCESS"
		if i == len(cfg.stages)-1 {
			switch b.Result {
			case "FAILURE":
				status = "FAILED"
			case "ABORTED":
				status = "ABORTED"
			}
		}
		stages[i] = apiStage{
			Name:            spec.name,
			Status:          status,
			StartTimeMillis: b.Timestamp + elapsed,
			DurationMillis:  dur,
		}
		elapsed += dur
	}
	return stages
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
		writeJSON(w, apiBuildList{Builds: fakeBuilds[name]})
		return
	}
	writeJSON(w, apiJobList{Jobs: []apiJob{}})
}

func (s *Server) handleStages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	buildNumStr := r.PathValue("buildnum")
	buildNum, _ := strconv.Atoi(buildNumStr)
	s.logger.Info("fake: stages", "name", name, "build", buildNumStr)
	writeJSON(w, apiWFDescribe{Stages: stagesForBuild(name, buildNum)})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
