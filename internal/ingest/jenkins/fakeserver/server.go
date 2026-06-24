package fakeserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	classWorkflowJob         = "org.jenkinsci.plugins.workflow.job.WorkflowJob"
	classMultibranchPipeline = "org.jenkinsci.plugins.workflow.multibranch.WorkflowMultiBranchProject"
	classGitBuildData        = "hudson.plugins.git.util.BuildData"
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
	RemoteUrls        []string     `json:"remoteUrls,omitempty"`
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
	// shared-lib/main: library CI — build + test, typically 1–2 min.
	"shared-lib/main": {
		stages:    []stageSpec{{"Checkout", 0.05}, {"Build", 0.45}, {"Test", 0.50}},
		durations: []int64{55_000, 70_000, 90_000},
	},
	// shared-lib/hotfix: same pipeline on the hotfix branch — slightly faster.
	"shared-lib/hotfix": {
		stages:    []stageSpec{{"Checkout", 0.05}, {"Build", 0.45}, {"Test", 0.50}},
		durations: []int64{48_000, 62_000, 85_000},
	},
}

// jobRemoteURL maps each fixture job to its Bitbucket Server remote URL.
// Project key and slug match the PROJ fixture in the fake Bitbucket server
// so that extractRepoID round-trips to "PROJ/backend-api", "PROJ/frontend-app",
// and "PROJ/shared-lib" — the same IDs referenced in the component YAML files.
var jobRemoteURL = map[string]string{
	"build-backend":     "https://bitbucket.example.com/scm/PROJ/backend-api.git",
	"test-backend":      "https://bitbucket.example.com/scm/PROJ/backend-api.git",
	"deploy-backend":    "https://bitbucket.example.com/scm/PROJ/backend-api.git",
	"build-frontend":    "https://bitbucket.example.com/scm/PROJ/frontend-app.git",
	"deploy-frontend":   "https://bitbucket.example.com/scm/PROJ/frontend-app.git",
	"shared-lib/main":   "https://bitbucket.example.com/scm/PROJ/shared-lib.git",
	"shared-lib/hotfix": "https://bitbucket.example.com/scm/PROJ/shared-lib.git",
}

// jobOrder fixes the iteration order of jobs so the fixture is deterministic.
var jobOrder = []string{
	"build-backend", "test-backend", "deploy-backend",
	"build-frontend", "deploy-frontend",
}

// multibranchPipelines lists root-level MultiBranchPipeline jobs and their branches.
var multibranchPipelines = map[string][]string{
	"shared-lib": {"main", "hotfix"},
}

// multibranchOrder fixes the iteration order of multibranch pipelines.
var multibranchOrder = []string{"shared-lib"}

// multibranchBuilds holds pre-generated builds for each branch job path.
var multibranchBuilds map[string][]apiBuild

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
	rootJobs = make([]apiJob, 0, len(jobOrder)+len(multibranchOrder))
	for _, name := range jobOrder {
		rootJobs = append(rootJobs, apiJob{Class: classWorkflowJob, Name: name})
	}
	for _, name := range multibranchOrder {
		rootJobs = append(rootJobs, apiJob{Class: classMultibranchPipeline, Name: name})
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
					RemoteUrls: []string{jobRemoteURL[name]},
				}},
			}
		}
		fakeBuilds[name] = builds
	}

	// Generate builds per branch for each multibranch pipeline using config durations.
	multibranchBuilds = make(map[string][]apiBuild)
	mbResults := [3]string{"SUCCESS", "FAILURE", "SUCCESS"}
	mbBranches := [3]string{"origin/main", "origin/main", "origin/hotfix"}
	const mbSpan = 7 * 24 * time.Hour
	for _, pipeline := range multibranchOrder {
		for branchIdx, branch := range multibranchPipelines[pipeline] {
			jobPath := pipeline + "/" + branch
			cfg := jobConfigs[jobPath]
			n := len(cfg.durations)
			if n == 0 {
				n = 3
			}
			builds := make([]apiBuild, n)
			for i := range builds {
				dur := int64(60_000)
				if i < len(cfg.durations) {
					dur = cfg.durations[i]
				}
				buildTime := epoch.Add(-mbSpan + time.Duration(i)*(mbSpan/time.Duration(n)))
				builds[i] = apiBuild{
					Number:    i + 1,
					Result:    mbResults[i%len(mbResults)],
					Timestamp: ms(buildTime),
					Duration:  dur,
					Actions: []apiBuildAction{{
						Class: classGitBuildData,
						LastBuiltRevision: &apiRevision{
							SHA1:   deterministicSHA(100+branchIdx, i),
							Branch: []apiBranch{{Name: mbBranches[i%len(mbBranches)]}},
						},
						RemoteUrls: []string{jobRemoteURL[jobPath]},
					}},
				}
			}
			multibranchBuilds[jobPath] = builds
		}
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

// stagesForBuild computes stage data for a named job and the given build.
// Stage durations are proportional to the total build duration; the final
// stage of a FAILURE or ABORTED build carries the matching status.
func stagesForBuild(jobName string, b apiBuild) []apiStage {
	cfg, ok := jobConfigs[jobName]
	if !ok {
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

// lastCompletedNumber returns the highest completed (non-in-progress, non-empty
// result) build number from the list, or (0, false) if none exist.
func lastCompletedNumber(builds []apiBuild) (int, bool) {
	max := 0
	found := false
	for _, b := range builds {
		if !b.InProgress && b.Result != "" && b.Number > max {
			max = b.Number
			found = true
		}
	}
	return max, found
}

// apiLastCompletedProbeResponse is the JSON shape for the lastCompletedBuild probe.
type apiLastCompletedProbeResponse struct {
	LastCompletedBuild *struct {
		Number int `json:"number"`
	} `json:"lastCompletedBuild"`
}

// Server is a fake Jenkins HTTP server. Each Server instance owns a mutable
// copy of the build fixtures so that AddBuild does not affect other instances.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	mu         sync.RWMutex
	builds     map[string][]apiBuild // mutable per-instance, most-recent-first
}

// New creates a new fake Jenkins Server listening on addr.
func New(addr string, logger *slog.Logger) *Server {
	// Copy the global fixture maps into per-instance mutable maps.
	builds := make(map[string][]apiBuild, len(fakeBuilds)+len(multibranchBuilds))
	for k, v := range fakeBuilds {
		cp := make([]apiBuild, len(v))
		copy(cp, v)
		builds[k] = cp
	}
	for k, v := range multibranchBuilds {
		cp := make([]apiBuild, len(v))
		copy(cp, v)
		builds[k] = cp
	}

	mux := http.NewServeMux()
	s := &Server{
		httpServer: &http.Server{Addr: addr, Handler: mux},
		logger:     logger,
		builds:     builds,
	}
	mux.HandleFunc("GET /api/json", s.handleRootJobs)
	mux.HandleFunc("GET /job/{name}/api/json", s.handleJobAPI)
	mux.HandleFunc("GET /job/{name}/{buildnum}/wfapi/describe", s.handleStages)
	mux.HandleFunc("GET /job/{pipeline}/job/{branch}/api/json", s.handleBranchJobAPI)
	mux.HandleFunc("GET /job/{pipeline}/job/{branch}/{buildnum}/wfapi/describe", s.handleBranchStages)
	return s
}

// AddBuild prepends b to jobPath's build list so it becomes the newest build
// returned by subsequent API requests. Safe to call concurrently with
// in-flight HTTP requests.
func (s *Server) AddBuild(jobPath string, b apiBuild) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.builds[jobPath] = append([]apiBuild{b}, s.builds[jobPath]...)
}

// NewBuild is a convenience helper that builds an apiBuild with the given
// number and result, a timestamp of now, and a 60-second duration.
func NewBuild(number int, result string) apiBuild {
	return apiBuild{
		Number:    number,
		Result:    result,
		Timestamp: time.Now().UnixMilli(),
		Duration:  60_000,
	}
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

	s.mu.RLock()
	builds := s.builds[name]
	s.mu.RUnlock()

	if strings.HasPrefix(tree, "lastCompletedBuild") {
		n, ok := lastCompletedNumber(builds)
		resp := apiLastCompletedProbeResponse{}
		if ok {
			resp.LastCompletedBuild = &struct {
				Number int `json:"number"`
			}{Number: n}
		}
		writeJSON(w, resp)
		return
	}
	if strings.HasPrefix(tree, "builds") {
		writeJSON(w, apiBuildList{Builds: builds})
		return
	}
	// Return branch jobs when this is a known multibranch pipeline.
	if branches, ok := multibranchPipelines[name]; ok {
		branchJobs := make([]apiJob, len(branches))
		for i, b := range branches {
			branchJobs[i] = apiJob{Class: classWorkflowJob, Name: b}
		}
		writeJSON(w, apiJobList{Jobs: branchJobs})
		return
	}
	writeJSON(w, apiJobList{Jobs: []apiJob{}})
}

func (s *Server) handleBranchJobAPI(w http.ResponseWriter, r *http.Request) {
	pipeline := r.PathValue("pipeline")
	branch := r.PathValue("branch")
	jobPath := pipeline + "/" + branch
	tree := r.URL.Query().Get("tree")
	s.logger.Info("fake: branch job api", "pipeline", pipeline, "branch", branch)

	s.mu.RLock()
	builds := s.builds[jobPath]
	s.mu.RUnlock()

	if strings.HasPrefix(tree, "lastCompletedBuild") {
		n, ok := lastCompletedNumber(builds)
		resp := apiLastCompletedProbeResponse{}
		if ok {
			resp.LastCompletedBuild = &struct {
				Number int `json:"number"`
			}{Number: n}
		}
		writeJSON(w, resp)
		return
	}
	writeJSON(w, apiBuildList{Builds: builds})
}

func (s *Server) handleBranchStages(w http.ResponseWriter, r *http.Request) {
	pipeline := r.PathValue("pipeline")
	branch := r.PathValue("branch")
	jobPath := pipeline + "/" + branch
	buildNumStr := r.PathValue("buildnum")
	buildNum, _ := strconv.Atoi(buildNumStr)
	s.logger.Info("fake: branch stages", "pipeline", pipeline, "branch", branch, "build", buildNumStr)

	s.mu.RLock()
	var b *apiBuild
	for i := range s.builds[jobPath] {
		if s.builds[jobPath][i].Number == buildNum {
			cp := s.builds[jobPath][i]
			b = &cp
			break
		}
	}
	s.mu.RUnlock()

	if b == nil {
		writeJSON(w, apiWFDescribe{Stages: nil})
		return
	}
	writeJSON(w, apiWFDescribe{Stages: stagesForBuild(jobPath, *b)})
}

func (s *Server) handleStages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	buildNumStr := r.PathValue("buildnum")
	buildNum, _ := strconv.Atoi(buildNumStr)
	s.logger.Info("fake: stages", "name", name, "build", buildNumStr)

	s.mu.RLock()
	var b *apiBuild
	for i := range s.builds[name] {
		if s.builds[name][i].Number == buildNum {
			cp := s.builds[name][i]
			b = &cp
			break
		}
	}
	s.mu.RUnlock()

	if b == nil {
		writeJSON(w, apiWFDescribe{Stages: nil})
		return
	}
	writeJSON(w, apiWFDescribe{Stages: stagesForBuild(name, *b)})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
