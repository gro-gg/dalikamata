package jenkins

import (
	"context"
	"io"
	"log/slog"
	"testing"
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
	if got[0] != "build-service" || got[1] != "deploy-service" {
		t.Errorf("jobs = %v, want [build-service deploy-service]", got)
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
	if got[0] != "team/api" || got[1] != "team/worker" {
		t.Errorf("jobs = %v, want [team/api team/worker]", got)
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
	if got[0] != "my-app/main" || got[1] != "my-app/feature-x" {
		t.Errorf("jobs = %v, want [my-app/main my-app/feature-x]", got)
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
	if len(got) != 1 || got[0] != "kept" {
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
