package jenkins

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeJenkinsClient is an in-package fake used for unit testing Crawler logic.
type fakeJenkinsClient struct {
	jobs          map[string][]apiJob
	builds        map[string][]apiBuild
	stages        map[string][]apiStage
	lastCompleted map[string]int // jobPath → newest completed build number; absent ⇒ (0, false)

	mu                 sync.Mutex
	lastCompletedCalls map[string]int // jobPath → number of probe calls
}

func (f *fakeJenkinsClient) GetJobs(_ context.Context, jobPath string) ([]apiJob, error) {
	return f.jobs[jobPath], nil
}

func (f *fakeJenkinsClient) GetBuilds(_ context.Context, jobPath string) ([]apiBuild, error) {
	return f.builds[jobPath], nil
}

func (f *fakeJenkinsClient) GetStages(_ context.Context, jobPath string, buildNumber int) ([]apiStage, error) {
	key := jobPath + "/" + itoa(buildNumber)
	return f.stages[key], nil
}

func (f *fakeJenkinsClient) GetLastCompletedBuildNumber(_ context.Context, jobPath string) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastCompletedCalls == nil {
		f.lastCompletedCalls = map[string]int{}
	}
	f.lastCompletedCalls[jobPath]++
	// Explicit override wins.
	if n, ok := f.lastCompleted[jobPath]; ok {
		return n, true, nil
	}
	// Fall back to computing the max completed build number from the builds map.
	max := 0
	found := false
	for _, b := range f.builds[jobPath] {
		if !b.InProgress && b.Result != "" && b.Number > max {
			max = b.Number
			found = true
		}
	}
	return max, found, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// ---- extractBranch ----------------------------------------------------------

func TestExtractBranch_StripsOriginPrefix(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{
				Class: "hudson.plugins.git.util.BuildData",
				LastBuiltRevision: &apiRevision{
					Branch: []apiBranch{{Name: "origin/main"}},
				},
			},
		},
	}
	got := extractBranch(b)
	if got != "main" {
		t.Errorf("extractBranch = %q, want %q", got, "main")
	}
}

func TestExtractBranch_NoOriginPrefix(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{
				Class: "hudson.plugins.git.util.BuildData",
				LastBuiltRevision: &apiRevision{
					Branch: []apiBranch{{Name: "feature/my-branch"}},
				},
			},
		},
	}
	got := extractBranch(b)
	if got != "feature/my-branch" {
		t.Errorf("extractBranch = %q, want %q", got, "feature/my-branch")
	}
}

func TestExtractBranch_NoMatchingAction(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{Class: "some.other.Action"},
		},
	}
	if got := extractBranch(b); got != "" {
		t.Errorf("extractBranch = %q, want empty", got)
	}
}

func TestExtractBranch_NoActions(t *testing.T) {
	if got := extractBranch(apiBuild{}); got != "" {
		t.Errorf("extractBranch = %q, want empty", got)
	}
}

func TestExtractBranch_NilRevision(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{Class: "hudson.plugins.git.util.BuildData", LastBuiltRevision: nil},
		},
	}
	if got := extractBranch(b); got != "" {
		t.Errorf("extractBranch = %q, want empty", got)
	}
}

func TestExtractBranch_EmptyBranchSlice(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{
				Class:             "hudson.plugins.git.util.BuildData",
				LastBuiltRevision: &apiRevision{Branch: []apiBranch{}},
			},
		},
	}
	if got := extractBranch(b); got != "" {
		t.Errorf("extractBranch = %q, want empty", got)
	}
}

// ---- extractCommitSHA -------------------------------------------------------

func TestExtractCommitSHA_ReturnsFirstMatchingSHA(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{Class: "unrelated.Action"},
			{
				Class:             "hudson.plugins.git.util.BuildData",
				LastBuiltRevision: &apiRevision{SHA1: "abc123"},
			},
		},
	}
	got := extractCommitSHA(b)
	if got != "abc123" {
		t.Errorf("extractCommitSHA = %q, want %q", got, "abc123")
	}
}

func TestExtractCommitSHA_NilRevision(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{
			{Class: "hudson.plugins.git.util.BuildData", LastBuiltRevision: nil},
		},
	}
	if got := extractCommitSHA(b); got != "" {
		t.Errorf("extractCommitSHA = %q, want empty", got)
	}
}

func TestExtractCommitSHA_NoActions(t *testing.T) {
	if got := extractCommitSHA(apiBuild{}); got != "" {
		t.Errorf("extractCommitSHA = %q, want empty", got)
	}
}

// ---- fakeCursors ------------------------------------------------------------

// fakeCursors is a map-backed Cursors for unit tests. It records every Save
// and Clear call so tests can assert correct cursor wiring.
type fakeCursors struct {
	mu    sync.Mutex
	data  map[string]int
	saves []struct {
		jobPath string
		number  int
	}
	clears []string
}

func newFakeCursors() *fakeCursors {
	return &fakeCursors{data: map[string]int{}}
}

func (f *fakeCursors) Load(_ context.Context) (map[string]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]int, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out, nil
}

func (f *fakeCursors) Save(_ context.Context, jobPath string, number int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[jobPath] = number
	f.saves = append(f.saves, struct {
		jobPath string
		number  int
	}{jobPath, number})
	return nil
}

func (f *fakeCursors) Clear(_ context.Context, jobPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, jobPath)
	f.clears = append(f.clears, jobPath)
	return nil
}

// ---- fakeCICDPublisher ------------------------------------------------------

type fakeCICDPublisher struct {
	workflows    []model.Workflow
	workflowRuns []model.WorkflowRun
	tasks        []model.WorkflowTask
}

func (f *fakeCICDPublisher) PublishWorkflow(_ context.Context, w model.Workflow) error {
	f.workflows = append(f.workflows, w)
	return nil
}
func (f *fakeCICDPublisher) PublishWorkflowRun(_ context.Context, r model.WorkflowRun) error {
	f.workflowRuns = append(f.workflowRuns, r)
	return nil
}
func (f *fakeCICDPublisher) PublishWorkflowTask(_ context.Context, t model.WorkflowTask) error {
	f.tasks = append(f.tasks, t)
	return nil
}

// ---- Crawl deduplication ----------------------------------------------------

func TestCrawl_MultibranchDeduplicatesWorkflow(t *testing.T) {
	// Two branch jobs under the same multibranch pipeline should produce exactly
	// one Workflow (ID "payments") but two sets of WorkflowRuns.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classMultibranchPipeline, Name: "payments"}},
			"payments": {
				{Class: classWorkflowJob, Name: "main"},
				{Class: classWorkflowJob, Name: "hotfix"},
			},
		},
		builds: map[string][]apiBuild{
			"payments/main": {
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
			"payments/hotfix": {
				{Number: 1, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 30_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), nil, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 {
		t.Fatalf("workflows published = %d, want 1", len(pub.workflows))
	}
	if pub.workflows[0].ID != "payments" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "payments")
	}
	if pub.workflows[0].Name != "payments" {
		t.Errorf("workflow Name = %q, want %q", pub.workflows[0].Name, "payments")
	}

	if len(pub.workflowRuns) != 2 {
		t.Fatalf("workflow runs published = %d, want 2", len(pub.workflowRuns))
	}
	for _, run := range pub.workflowRuns {
		if run.WorkflowID != "payments" {
			t.Errorf("run WorkflowID = %q, want %q", run.WorkflowID, "payments")
		}
	}
	// Run IDs must retain the full path to stay unique across branches.
	ids := map[string]bool{pub.workflowRuns[0].ID: true, pub.workflowRuns[1].ID: true}
	if !ids["payments/main/1"] || !ids["payments/hotfix/1"] {
		t.Errorf("run IDs = %v, want [payments/main/1 payments/hotfix/1]",
			[]string{pub.workflowRuns[0].ID, pub.workflowRuns[1].ID})
	}
}

func TestCrawl_PlainJobNotStripped(t *testing.T) {
	// A plain WorkflowJob (not inside a MultibranchPipeline) must keep its full
	// path as the workflow ID.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{
			"build-backend": {{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000}},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	if err := NewCrawler(client, pub, newFakeCursors(), nil, nil, discardLogger()).Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(pub.workflows) != 1 || pub.workflows[0].ID != "build-backend" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "build-backend")
	}
}

// ---- extractRepoID ----------------------------------------------------------

func TestExtractRepoID_PrefersHigherSHAVariety(t *testing.T) {
	// Shared library appears first but has the same SHA in every build (pinned).
	// App repo SHA changes each build → extractRepoID must pick the app repo.
	builds := []apiBuild{
		{Actions: []apiBuildAction{
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "libSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/shared-lib.git"}},
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "appSHA1"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/backend.git"}},
		}},
		{Actions: []apiBuildAction{
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "libSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/shared-lib.git"}},
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "appSHA2"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/backend.git"}},
		}},
	}
	got := extractRepoID(builds)
	if got != "PROJ/backend" {
		t.Errorf("extractRepoID = %q, want %q", got, "PROJ/backend")
	}
}

func TestExtractRepoID_FallsBackToLastBuildDataOnSingleBuild(t *testing.T) {
	// Single build: SHA variety cannot distinguish repos. Must return the last
	// BuildData URL, not the first (shared lib appears first in actions).
	builds := []apiBuild{
		{Actions: []apiBuildAction{
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "libSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/shared-lib.git"}},
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "appSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/backend.git"}},
		}},
	}
	got := extractRepoID(builds)
	if got != "PROJ/backend" {
		t.Errorf("extractRepoID = %q, want %q", got, "PROJ/backend")
	}
}

func TestExtractRepoID_FallsBackToLastBuildDataWhenAllSHAsSame(t *testing.T) {
	// Multiple builds, but all repos have the same SHA in every build (e.g. a
	// hotfix pipeline deploying a fixed tag). Cannot distinguish by SHA variety
	// → fall back to the last BuildData URL.
	builds := []apiBuild{
		{Actions: []apiBuildAction{
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "libSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/shared-lib.git"}},
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "appSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/backend.git"}},
		}},
		{Actions: []apiBuildAction{
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "libSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/shared-lib.git"}},
			{Class: classGitBuildData, LastBuiltRevision: &apiRevision{SHA1: "appSHA"}, RemoteUrls: []string{"https://bb.example.com/scm/proj/backend.git"}},
		}},
	}
	got := extractRepoID(builds)
	if got != "PROJ/backend" {
		t.Errorf("extractRepoID = %q, want %q", got, "PROJ/backend")
	}
}

func TestExtractRepoID_NoBuildData(t *testing.T) {
	builds := []apiBuild{{Actions: []apiBuildAction{{Class: "unrelated.Action"}}}}
	if got := extractRepoID(builds); got != "" {
		t.Errorf("extractRepoID = %q, want empty", got)
	}
}

func TestExtractRepoID_EmptyBuilds(t *testing.T) {
	if got := extractRepoID(nil); got != "" {
		t.Errorf("extractRepoID = %q, want empty", got)
	}
}

// ---- repoIDFromURL ----------------------------------------------------------

func TestRepoIDFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"bitbucket server https", "https://bitbucket.example.com/scm/ACME/backend.git", "ACME/backend"},
		{"bitbucket server https lowercase project", "https://bitbucket.example.com/scm/acme/backend.git", "ACME/backend"},
		{"bitbucket server ssh", "ssh://git@bitbucket.example.com/ACME/backend.git", "ACME/backend"},
		{"github https", "https://github.com/org/repo.git", "org/repo"},
		{"github no .git suffix", "https://github.com/org/repo", "org/repo"},
		{"scp-style git", "git@github.com:org/repo.git", "org/repo"},
		{"empty string", "", ""},
		{"single segment", "https://example.com/repo.git", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := repoIDFromURL(tc.url)
			if got != tc.want {
				t.Errorf("repoIDFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// ---- discoverJobs -----------------------------------------------------------

func TestDiscoverJobs_WorkflowJobsReturned(t *testing.T) {
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {
				{Class: classWorkflowJob, Name: "build-service"},
				{Class: classWorkflowJob, Name: "deploy-service"},
			},
		},
	}
	c := &Crawler{client: client, logger: discardLogger()}
	got, err := c.discoverJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("discoverJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d jobs, want 2", len(got))
	}
	if got[0].path != "build-service" || got[1].path != "deploy-service" {
		t.Errorf("paths = [%s %s], want [build-service deploy-service]", got[0].path, got[1].path)
	}
	if got[0].isBranch || got[1].isBranch {
		t.Errorf("plain workflow jobs must not be marked as branch jobs")
	}
}

func TestDiscoverJobs_RecursesIntoFolder(t *testing.T) {
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {
				{Class: classFolder, Name: "team"},
			},
			"team": {
				{Class: classWorkflowJob, Name: "api"},
				{Class: classWorkflowJob, Name: "worker"},
			},
		},
	}
	c := &Crawler{client: client, logger: discardLogger()}
	got, err := c.discoverJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("discoverJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d jobs, want 2", len(got))
	}
	if got[0].path != "team/api" || got[1].path != "team/worker" {
		t.Errorf("paths = [%s %s], want [team/api team/worker]", got[0].path, got[1].path)
	}
	if got[0].isBranch || got[1].isBranch {
		t.Errorf("folder children must not be marked as branch jobs")
	}
}

func TestDiscoverJobs_RecursesIntoMultibranchPipeline(t *testing.T) {
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {
				{Class: classMultibranchPipeline, Name: "my-app"},
			},
			"my-app": {
				{Class: classWorkflowJob, Name: "main"},
				{Class: classWorkflowJob, Name: "feature-x"},
			},
		},
	}
	c := &Crawler{client: client, logger: discardLogger()}
	got, err := c.discoverJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("discoverJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d jobs, want 2", len(got))
	}
	if got[0].path != "my-app/main" || got[1].path != "my-app/feature-x" {
		t.Errorf("paths = [%s %s], want [my-app/main my-app/feature-x]", got[0].path, got[1].path)
	}
	if !got[0].isBranch || !got[1].isBranch {
		t.Errorf("multibranch pipeline children must be marked as branch jobs")
	}
}

func TestDiscoverJobs_UnknownClassIgnored(t *testing.T) {
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {
				{Class: "some.unknown.JobType", Name: "ignored"},
				{Class: classWorkflowJob, Name: "kept"},
			},
		},
	}
	c := &Crawler{client: client, logger: discardLogger()}
	got, err := c.discoverJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("discoverJobs: %v", err)
	}
	if len(got) != 1 || got[0].path != "kept" {
		t.Errorf("jobs = %v, want [kept]", got)
	}
}

func TestDiscoverJobs_EmptyRoot(t *testing.T) {
	client := &fakeJenkinsClient{jobs: map[string][]apiJob{"": {}}}
	c := &Crawler{client: client, logger: discardLogger()}
	got, err := c.discoverJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("discoverJobs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d jobs, want 0", len(got))
	}
}

// ---- classifyConfiguredJobs / explicit-jobs path ----------------------------

func TestCrawl_ExplicitBranchJobStripsBranch(t *testing.T) {
	// Jobs: ["shared-lib/main"] — parent "shared-lib" is a MultibranchPipeline.
	// Workflow should be published as "shared-lib", not "shared-lib/main".
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classMultibranchPipeline, Name: "shared-lib"}},
		},
		builds: map[string][]apiBuild{
			"shared-lib/main": {
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"shared-lib/main"}, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 {
		t.Fatalf("workflows published = %d, want 1", len(pub.workflows))
	}
	if pub.workflows[0].ID != "shared-lib" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "shared-lib")
	}
	if pub.workflows[0].Name != "shared-lib" {
		t.Errorf("workflow Name = %q, want %q", pub.workflows[0].Name, "shared-lib")
	}
	if len(pub.workflowRuns) != 1 {
		t.Fatalf("workflow runs published = %d, want 1", len(pub.workflowRuns))
	}
	if pub.workflowRuns[0].WorkflowID != "shared-lib" {
		t.Errorf("run WorkflowID = %q, want %q", pub.workflowRuns[0].WorkflowID, "shared-lib")
	}
	if pub.workflowRuns[0].ID != "shared-lib/main/1" {
		t.Errorf("run ID = %q, want %q", pub.workflowRuns[0].ID, "shared-lib/main/1")
	}
}

func TestCrawl_ExplicitMultipleBranchesSamePipeline(t *testing.T) {
	// Jobs: ["shared-lib/main", "shared-lib/hotfix"] — both are branches of one
	// MultibranchPipeline. Should produce exactly one Workflow and two runs.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classMultibranchPipeline, Name: "shared-lib"}},
		},
		builds: map[string][]apiBuild{
			"shared-lib/main": {
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
			"shared-lib/hotfix": {
				{Number: 1, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 30_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"shared-lib/main", "shared-lib/hotfix"}, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 {
		t.Fatalf("workflows published = %d, want 1", len(pub.workflows))
	}
	if pub.workflows[0].ID != "shared-lib" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "shared-lib")
	}
	if len(pub.workflowRuns) != 2 {
		t.Fatalf("workflow runs published = %d, want 2", len(pub.workflowRuns))
	}
	for _, run := range pub.workflowRuns {
		if run.WorkflowID != "shared-lib" {
			t.Errorf("run WorkflowID = %q, want %q", run.WorkflowID, "shared-lib")
		}
	}
	ids := map[string]bool{pub.workflowRuns[0].ID: true, pub.workflowRuns[1].ID: true}
	if !ids["shared-lib/main/1"] || !ids["shared-lib/hotfix/1"] {
		t.Errorf("run IDs = %v, want [shared-lib/main/1 shared-lib/hotfix/1]",
			[]string{pub.workflowRuns[0].ID, pub.workflowRuns[1].ID})
	}
}

func TestCrawl_ExplicitNestedBranchJobStripsBranch(t *testing.T) {
	// Jobs: ["team/shared-lib/main"] — parent "shared-lib" is nested inside folder
	// "team". GetJobs("team") should reveal it as a MultibranchPipeline.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"team": {{Class: classMultibranchPipeline, Name: "shared-lib"}},
		},
		builds: map[string][]apiBuild{
			"team/shared-lib/main": {
				{Number: 2, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 45_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"team/shared-lib/main"}, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 {
		t.Fatalf("workflows published = %d, want 1", len(pub.workflows))
	}
	if pub.workflows[0].ID != "team/shared-lib" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "team/shared-lib")
	}
}

func TestCrawl_ExplicitPlainJobNotStripped(t *testing.T) {
	// Jobs: ["build-backend"] — root-level plain WorkflowJob. No parent lookup
	// should occur and the workflow ID must keep its full path.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{},
		builds: map[string][]apiBuild{
			"build-backend": {{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000}},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"build-backend"}, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 || pub.workflows[0].ID != "build-backend" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "build-backend")
	}
}

func TestCrawl_ExplicitJobParentNotMultibranch(t *testing.T) {
	// Jobs: ["team/api"] — parent "team" is a Folder, not a MultibranchPipeline.
	// Workflow ID must not be stripped.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classFolder, Name: "team"}},
		},
		builds: map[string][]apiBuild{
			"team/api": {{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000}},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, newFakeCursors(), []string{"team/api"}, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 || pub.workflows[0].ID != "team/api" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "team/api")
	}
}

// ---- incremental cursor tests -----------------------------------------------

func TestCrawl_BuildCursorAdvances(t *testing.T) {
	// First crawl: cursor is 0, all 3 builds published, cursor saved to 3.
	// Second crawl: probe returns same value as cursor → no builds published.
	// After adding a 4th build: probe returns 4 → only build 4 published.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{
			"build-backend": {
				{Number: 3, Result: "SUCCESS", Timestamp: 3_000_000, Duration: 60_000},
				{Number: 2, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 60_000},
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	store := newFakeCursors()
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, store, nil, nil, discardLogger())

	// First crawl: all 3 builds published, cursor saved as 3.
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 1: %v", err)
	}
	if len(pub.workflowRuns) != 3 {
		t.Fatalf("after crawl 1: runs = %d, want 3", len(pub.workflowRuns))
	}
	if store.data["build-backend"] != 3 {
		t.Errorf("cursor after crawl 1 = %d, want 3", store.data["build-backend"])
	}

	// Second crawl: probe returns 3 == cursor → GetBuilds not called, 0 new runs.
	pub2 := &fakeCICDPublisher{}
	crawler.publisher = pub2
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 2: %v", err)
	}
	if len(pub2.workflowRuns) != 0 {
		t.Errorf("after crawl 2 (no new builds): runs = %d, want 0", len(pub2.workflowRuns))
	}
	if len(pub2.workflows) != 0 {
		t.Errorf("after crawl 2 (no new builds): workflows = %d, want 0", len(pub2.workflows))
	}

	// Add build #4; third crawl publishes only the new build.
	client.mu.Lock()
	client.builds["build-backend"] = append(
		[]apiBuild{{Number: 4, Result: "SUCCESS", Timestamp: 4_000_000, Duration: 60_000}},
		client.builds["build-backend"]...,
	)
	client.mu.Unlock()

	pub3 := &fakeCICDPublisher{}
	crawler.publisher = pub3
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 3: %v", err)
	}
	if len(pub3.workflowRuns) != 1 {
		t.Fatalf("after crawl 3 (one new build): runs = %d, want 1", len(pub3.workflowRuns))
	}
	if pub3.workflowRuns[0].Number != 4 {
		t.Errorf("new run number = %d, want 4", pub3.workflowRuns[0].Number)
	}
	if store.data["build-backend"] != 4 {
		t.Errorf("cursor after crawl 3 = %d, want 4", store.data["build-backend"])
	}
}

func TestCrawl_CursorHydratedFromStore(t *testing.T) {
	// Pre-populate the cursor store (simulates restart with persisted cursor).
	// First crawl should use that cursor and publish 0 runs since probe == cursor.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{
			"build-backend": {
				{Number: 3, Result: "SUCCESS", Timestamp: 3_000_000, Duration: 60_000},
				{Number: 2, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 60_000},
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	store := newFakeCursors()
	store.data["build-backend"] = 3 // simulate persisted cursor from prior run

	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, store, nil, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(pub.workflowRuns) != 0 {
		t.Errorf("runs = %d, want 0 (cursor covers all existing builds)", len(pub.workflowRuns))
	}

	// After adding a new build, second crawl picks it up.
	client.mu.Lock()
	client.builds["build-backend"] = append(
		[]apiBuild{{Number: 4, Result: "SUCCESS", Timestamp: 4_000_000, Duration: 60_000}},
		client.builds["build-backend"]...,
	)
	client.mu.Unlock()

	pub2 := &fakeCICDPublisher{}
	crawler.publisher = pub2
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 2: %v", err)
	}
	if len(pub2.workflowRuns) != 1 || pub2.workflowRuns[0].Number != 4 {
		t.Errorf("runs after new build = %d, want 1 with number 4", len(pub2.workflowRuns))
	}
}

func TestCrawl_CursorClearedOnReset(t *testing.T) {
	// Cursor is ahead of lastCompletedBuild (job was reset). Cursor should be
	// cleared and all current builds re-fetched.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{
			"build-backend": {
				{Number: 2, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 60_000},
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	store := newFakeCursors()
	store.data["build-backend"] = 99 // stale cursor ahead of actual max

	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, store, nil, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(store.clears) == 0 || store.clears[0] != "build-backend" {
		t.Errorf("expected cursor to be cleared; clears = %v", store.clears)
	}
	if len(pub.workflowRuns) != 2 {
		t.Errorf("runs after reset = %d, want 2 (full refetch)", len(pub.workflowRuns))
	}
	if store.data["build-backend"] != 2 {
		t.Errorf("cursor after reset+refetch = %d, want 2", store.data["build-backend"])
	}
}

func TestCrawl_NoWorkflowRepublishWhenNoNewBuilds(t *testing.T) {
	// Second crawl with no new builds must not re-publish the Workflow entity.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{
			"build-backend": {
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	store := newFakeCursors()
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, store, nil, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 1: %v", err)
	}
	if len(pub.workflows) != 1 {
		t.Fatalf("crawl 1: workflows = %d, want 1", len(pub.workflows))
	}

	pub2 := &fakeCICDPublisher{}
	crawler.publisher = pub2
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 2: %v", err)
	}
	if len(pub2.workflows) != 0 {
		t.Errorf("crawl 2 (no new builds): workflows = %d, want 0", len(pub2.workflows))
	}
}

func TestCrawl_InProgressBuildDoesNotAdvanceCursor(t *testing.T) {
	// An in-progress build must not be published and must not advance the cursor.
	// When it completes on the next tick, it should then be published.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{
			"build-backend": {
				{Number: 3, InProgress: true, Result: ""},
				{Number: 2, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 60_000},
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
		},
		// Probe should see lastCompleted as 2 (build 3 is in-progress).
		lastCompleted: map[string]int{"build-backend": 2},
		stages:        map[string][]apiStage{},
	}
	store := newFakeCursors()
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, store, nil, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 1: %v", err)
	}
	if len(pub.workflowRuns) != 2 {
		t.Fatalf("crawl 1: runs = %d, want 2 (builds 1+2 only)", len(pub.workflowRuns))
	}
	if store.data["build-backend"] != 2 {
		t.Errorf("cursor after crawl 1 = %d, want 2 (in-progress build must not advance cursor)", store.data["build-backend"])
	}

	// Build 3 completes between ticks.
	client.mu.Lock()
	client.builds["build-backend"] = []apiBuild{
		{Number: 3, Result: "SUCCESS", Timestamp: 3_000_000, Duration: 60_000},
		{Number: 2, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 60_000},
		{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
	}
	client.lastCompleted = map[string]int{"build-backend": 3}
	client.mu.Unlock()

	pub2 := &fakeCICDPublisher{}
	crawler.publisher = pub2
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 2: %v", err)
	}
	if len(pub2.workflowRuns) != 1 || pub2.workflowRuns[0].Number != 3 {
		t.Errorf("crawl 2: runs = %d, want 1 (build 3 now complete)", len(pub2.workflowRuns))
	}
	if store.data["build-backend"] != 3 {
		t.Errorf("cursor after crawl 2 = %d, want 3", store.data["build-backend"])
	}
}

func TestCrawl_MultibranchPerBranchCursors(t *testing.T) {
	// Two explicit branches of the same MM pipeline should get independent
	// cursors keyed by their full job paths.
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classMultibranchPipeline, Name: "shared-lib"}},
		},
		builds: map[string][]apiBuild{
			"shared-lib/main": {
				{Number: 2, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 60_000},
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000},
			},
			"shared-lib/hotfix": {
				{Number: 1, Result: "SUCCESS", Timestamp: 1_500_000, Duration: 60_000},
			},
		},
		stages: map[string][]apiStage{},
	}
	store := newFakeCursors()
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, store, []string{"shared-lib/main", "shared-lib/hotfix"}, nil, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 || pub.workflows[0].ID != "shared-lib" {
		t.Fatalf("workflows = %v, want one workflow 'shared-lib'", pub.workflows)
	}
	if len(pub.workflowRuns) != 3 {
		t.Fatalf("runs = %d, want 3", len(pub.workflowRuns))
	}
	// Cursors are keyed by full job path, not by pipelinePath.
	if store.data["shared-lib/main"] != 2 {
		t.Errorf("cursor shared-lib/main = %d, want 2", store.data["shared-lib/main"])
	}
	if store.data["shared-lib/hotfix"] != 1 {
		t.Errorf("cursor shared-lib/hotfix = %d, want 1", store.data["shared-lib/hotfix"])
	}

	// Second crawl: no new builds → no re-publishes.
	pub2 := &fakeCICDPublisher{}
	crawler.publisher = pub2
	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl 2: %v", err)
	}
	if len(pub2.workflows) != 0 {
		t.Errorf("crawl 2: workflows = %d, want 0", len(pub2.workflows))
	}
	if len(pub2.workflowRuns) != 0 {
		t.Errorf("crawl 2: runs = %d, want 0", len(pub2.workflowRuns))
	}
}

// ---- branchForJob -----------------------------------------------------------

func TestBranchForJob_MultibranchUsesJobPath(t *testing.T) {
	// BuildData reports "master" — the detached-HEAD artifact common in
	// Multi-branch pipelines. Branch must come from the job path, not BuildData.
	b := apiBuild{
		Actions: []apiBuildAction{{
			Class:             classGitBuildData,
			LastBuiltRevision: &apiRevision{Branch: []apiBranch{{Name: "origin/master"}}},
		}},
	}
	got := branchForJob("payments/feature-foo", "payments", b)
	if got != "feature-foo" {
		t.Errorf("branchForJob = %q, want %q", got, "feature-foo")
	}
}

func TestBranchForJob_NestedMultibranchUsesJobPath(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{{
			Class:             classGitBuildData,
			LastBuiltRevision: &apiRevision{Branch: []apiBranch{{Name: "origin/master"}}},
		}},
	}
	got := branchForJob("team/shared-lib/main", "team/shared-lib", b)
	if got != "main" {
		t.Errorf("branchForJob = %q, want %q", got, "main")
	}
}

func TestBranchForJob_RegularJobUsesBuildData(t *testing.T) {
	b := apiBuild{
		Actions: []apiBuildAction{{
			Class:             classGitBuildData,
			LastBuiltRevision: &apiRevision{Branch: []apiBranch{{Name: "origin/main"}}},
		}},
	}
	got := branchForJob("build-backend", "build-backend", b)
	if got != "main" {
		t.Errorf("branchForJob = %q, want %q", got, "main")
	}
}

// ---- branch field on WorkflowRun for multibranch pipelines ------------------

func TestCrawl_MultibranchBranchFromJobPath(t *testing.T) {
	// BuildData for both branch jobs incorrectly reports "origin/master" — the
	// detached-HEAD artifact. Each WorkflowRun.Branch must be taken from the job
	// path, not from BuildData.
	staleBuildData := apiBuildAction{
		Class:             classGitBuildData,
		LastBuiltRevision: &apiRevision{Branch: []apiBranch{{Name: "origin/master"}}},
	}
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classMultibranchPipeline, Name: "payments"}},
			"payments": {
				{Class: classWorkflowJob, Name: "main"},
				{Class: classWorkflowJob, Name: "feature-foo"},
			},
		},
		builds: map[string][]apiBuild{
			"payments/main": {
				{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 60_000,
					Actions: []apiBuildAction{staleBuildData}},
			},
			"payments/feature-foo": {
				{Number: 1, Result: "SUCCESS", Timestamp: 2_000_000, Duration: 30_000,
					Actions: []apiBuildAction{staleBuildData}},
			},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	if err := NewCrawler(client, pub, newFakeCursors(), nil, nil, discardLogger()).Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflowRuns) != 2 {
		t.Fatalf("workflow runs = %d, want 2", len(pub.workflowRuns))
	}
	branches := map[string]string{}
	for _, r := range pub.workflowRuns {
		branches[r.ID] = r.Branch
	}
	if branches["payments/main/1"] != "main" {
		t.Errorf("payments/main/1 branch = %q, want %q", branches["payments/main/1"], "main")
	}
	if branches["payments/feature-foo/1"] != "feature-foo" {
		t.Errorf("payments/feature-foo/1 branch = %q, want %q", branches["payments/feature-foo/1"], "feature-foo")
	}
}

// TestPublishWorkflow_EmptyRepoIDPublishesWithWarning verifies that when
// extractRepoID returns "" (no BuildData action present) and no repo_overrides
// entry exists, PublishWorkflow is still called (the record is indexed) but
// with an empty RepoID — the ownership chain will report "missing_repo_id".
func TestPublishWorkflow_EmptyRepoIDPublishesWithWarning(t *testing.T) {
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classWorkflowJob, Name: "no-git-info"}},
		},
		// Build has no BuildData action — extractRepoID will return "".
		builds: map[string][]apiBuild{
			"no-git-info": {{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 10_000}},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	if err := NewCrawler(client, pub, newFakeCursors(), nil, nil, discardLogger()).Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(pub.workflows) != 1 {
		t.Fatalf("workflows published = %d, want 1", len(pub.workflows))
	}
	if pub.workflows[0].RepoID != "" {
		t.Errorf("workflow RepoID = %q, want empty (no BuildData)", pub.workflows[0].RepoID)
	}
	if len(pub.workflowRuns) != 1 {
		t.Errorf("workflow runs = %d, want 1", len(pub.workflowRuns))
	}
}

// TestRepoOverride_PopulatesRepoID verifies that when extractRepoID returns ""
// but a repo_overrides entry exists for the pipeline, PublishWorkflow IS called
// with the override repoID.
func TestRepoOverride_PopulatesRepoID(t *testing.T) {
	client := &fakeJenkinsClient{
		jobs: map[string][]apiJob{
			"": {{Class: classWorkflowJob, Name: "no-git-info"}},
		},
		builds: map[string][]apiBuild{
			"no-git-info": {{Number: 1, Result: "SUCCESS", Timestamp: 1_000_000, Duration: 10_000}},
		},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	overrides := map[string]string{"no-git-info": "PROJ/my-repo"}
	if err := NewCrawler(client, pub, newFakeCursors(), nil, overrides, discardLogger()).Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(pub.workflows) != 1 {
		t.Fatalf("workflows published = %d, want 1", len(pub.workflows))
	}
	if pub.workflows[0].RepoID != "PROJ/my-repo" {
		t.Errorf("workflow RepoID = %q, want PROJ/my-repo", pub.workflows[0].RepoID)
	}
}
