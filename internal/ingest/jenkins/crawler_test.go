package jenkins

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeJenkinsClient is an in-package fake used for unit testing Crawler logic.
type fakeJenkinsClient struct {
	jobs   map[string][]apiJob
	builds map[string][]apiBuild
	stages map[string][]apiStage
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
	crawler := NewCrawler(client, pub, nil, discardLogger())

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
		jobs:   map[string][]apiJob{"": {{Class: classWorkflowJob, Name: "build-backend"}}},
		builds: map[string][]apiBuild{"build-backend": {}},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	if err := NewCrawler(client, pub, nil, discardLogger()).Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(pub.workflows) != 1 || pub.workflows[0].ID != "build-backend" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "build-backend")
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
	crawler := NewCrawler(client, pub, []string{"shared-lib/main"}, discardLogger())

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
	crawler := NewCrawler(client, pub, []string{"shared-lib/main", "shared-lib/hotfix"}, discardLogger())

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
	crawler := NewCrawler(client, pub, []string{"team/shared-lib/main"}, discardLogger())

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
		jobs:   map[string][]apiJob{},
		builds: map[string][]apiBuild{"build-backend": {}},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, []string{"build-backend"}, discardLogger())

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
		builds: map[string][]apiBuild{"team/api": {}},
		stages: map[string][]apiStage{},
	}
	pub := &fakeCICDPublisher{}
	crawler := NewCrawler(client, pub, []string{"team/api"}, discardLogger())

	if err := crawler.Crawl(context.Background()); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if len(pub.workflows) != 1 || pub.workflows[0].ID != "team/api" {
		t.Errorf("workflow ID = %q, want %q", pub.workflows[0].ID, "team/api")
	}
}
