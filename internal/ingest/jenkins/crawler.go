package jenkins

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/domain/model"
)

const (
	classWorkflowJob         = "org.jenkinsci.plugins.workflow.job.WorkflowJob"
	classFolder              = "com.cloudbees.hudson.plugins.folder.Folder"
	classMultibranchPipeline = "org.jenkinsci.plugins.workflow.multibranch.WorkflowMultiBranchProject"
	classGitBuildData        = "hudson.plugins.git.util.BuildData"
)

// jobEntry is a discovered Jenkins workflow job and the context needed to
// derive its canonical pipeline identity.
type jobEntry struct {
	path     string
	isBranch bool // true when discovered as a direct child of a MultibranchPipeline
}

// pipelinePath returns the canonical workflow identifier for an entry.
// For branch jobs it strips the branch leaf so all branches of the same
// pipeline share one ID, e.g. "payments/main" → "payments".
func pipelinePath(e jobEntry) string {
	if e.isBranch {
		return path.Dir(e.path)
	}
	return e.path
}

type Crawler struct {
	client    JenkinsClient
	publisher domain.CICDPublisher
	store     Cursors
	jobs      []string
	logger    *slog.Logger

	mu      sync.Mutex
	cursors map[string]int // jobPath → highest completed build number published
	loaded  bool           // true after the first Load from the store
}

func NewCrawler(client JenkinsClient, publisher domain.CICDPublisher, store Cursors, jobs []string, logger *slog.Logger) *Crawler {
	return &Crawler{
		client:    client,
		publisher: publisher,
		store:     store,
		jobs:      jobs,
		cursors:   map[string]int{},
		logger:    logger.With("component", "jenkins-crawler"),
	}
}

func (c *Crawler) Crawl(ctx context.Context) error {
	// Hydrate per-job cursors from the persistent store on the very first tick.
	// On failure we log and continue with an empty map (full refetch), which is
	// safe because downstream storage is idempotent.
	c.mu.Lock()
	if !c.loaded {
		c.mu.Unlock()
		loaded, err := c.store.Load(ctx)
		c.mu.Lock()
		if err != nil {
			c.logger.Warn("loading build cursors failed; starting fresh", "error", err)
			loaded = map[string]int{}
		}
		c.cursors = loaded
		c.loaded = true
	}
	c.mu.Unlock()

	var entries []jobEntry
	if len(c.jobs) == 0 {
		c.logger.Info("No jobs specified, discovering all jobs")
		var err error
		entries, err = c.discoverJobs(ctx, "")
		if err != nil {
			return fmt.Errorf("discovering jobs: %w", err)
		}
		c.logger.Info("Discovered jobs", "count", len(entries))
	} else {
		var err error
		entries, err = c.classifyConfiguredJobs(ctx)
		if err != nil {
			return fmt.Errorf("classifying configured jobs: %w", err)
		}
		c.logger.Info("Classified configured jobs", "count", len(entries))
	}

	// Group entries by their canonical pipeline path so that all branches of a
	// MultibranchPipeline share one Workflow and are not published separately.
	// Use a slice to preserve discovery order for deterministic output.
	type entryWithBuilds struct {
		entry  jobEntry
		builds []apiBuild
	}
	type pipelineGroup struct {
		jobs []entryWithBuilds
	}
	groups := make(map[string]*pipelineGroup)
	var order []string
	for _, entry := range entries {
		pp := pipelinePath(entry)
		if _, seen := groups[pp]; !seen {
			groups[pp] = &pipelineGroup{}
			order = append(order, pp)
		}

		c.mu.Lock()
		cursor := c.cursors[entry.path]
		c.mu.Unlock()

		// Probe the last completed build to decide whether a fetch is needed.
		last, hasBuilds, err := c.client.GetLastCompletedBuildNumber(ctx, entry.path)
		if err != nil {
			c.logger.Error("probing last completed build", "job", entry.path, "error", err)
			// Fall through: fetch anyway so we don't silently skip jobs on transient errors.
		} else if !hasBuilds {
			c.logger.Debug("no completed builds yet", "job", entry.path)
			groups[pp].jobs = append(groups[pp].jobs, entryWithBuilds{entry: entry})
			continue
		} else if last < cursor {
			// Job was reset or builds were purged. Clear the cursor and full-refetch.
			c.logger.Warn("last completed build is behind cursor; clearing cursor for full refetch",
				"job", entry.path, "last", last, "cursor", cursor)
			if clearErr := c.store.Clear(ctx, entry.path); clearErr != nil {
				c.logger.Error("clearing cursor", "job", entry.path, "error", clearErr)
			}
			c.mu.Lock()
			delete(c.cursors, entry.path)
			cursor = 0
			c.mu.Unlock()
		} else if last == cursor {
			c.logger.Debug("no new builds", "job", entry.path, "cursor", cursor)
			groups[pp].jobs = append(groups[pp].jobs, entryWithBuilds{entry: entry})
			continue
		}

		allBuilds, err := c.client.GetBuilds(ctx, entry.path)
		if err != nil {
			c.logger.Error("fetching builds", "job", entry.path, "error", err)
		}

		// Keep only completed builds newer than the cursor.
		var newBuilds []apiBuild
		for _, b := range allBuilds {
			if b.InProgress || b.Result == "" {
				continue
			}
			if b.Number > cursor {
				newBuilds = append(newBuilds, b)
			}
		}

		groups[pp].jobs = append(groups[pp].jobs, entryWithBuilds{entry: entry, builds: newBuilds})
	}

	for _, pp := range order {
		group := groups[pp]

		// Collect all new builds across branch jobs to derive the shared repo ID.
		var allNewBuilds []apiBuild
		for _, j := range group.jobs {
			allNewBuilds = append(allNewBuilds, j.builds...)
		}

		// Skip the workflow re-publish when there is nothing new for this pipeline.
		if len(allNewBuilds) == 0 {
			continue
		}

		workflow := model.Workflow{
			ID:     pp,
			Name:   pp,
			RepoID: extractRepoID(allNewBuilds),
		}
		if err := c.publisher.PublishWorkflow(ctx, workflow); err != nil {
			c.logger.Error("publishing workflow", "pipeline", pp, "error", err)
		}

		for _, j := range group.jobs {
			if len(j.builds) == 0 {
				continue
			}
			if err := c.crawlJob(ctx, j.entry.path, pp, j.builds); err != nil {
				c.logger.Error("crawling job", "job", j.entry.path, "error", err)
				continue
			}
			// Advance the cursor to the highest build number published for this
			// job. Save only after successful publish — a save failure means the
			// next tick re-fetches rather than silently skipping.
			maxPublished := 0
			for _, b := range j.builds {
				if b.Number > maxPublished {
					maxPublished = b.Number
				}
			}
			if saveErr := c.store.Save(ctx, j.entry.path, maxPublished); saveErr != nil {
				c.logger.Error("saving build cursor", "job", j.entry.path, "number", maxPublished, "error", saveErr)
				continue
			}
			c.mu.Lock()
			c.cursors[j.entry.path] = maxPublished
			c.mu.Unlock()
		}
	}
	return nil
}

func (c *Crawler) discoverJobs(ctx context.Context, folderPath string) ([]jobEntry, error) {
	jobs, err := c.client.GetJobs(ctx, folderPath)
	if err != nil {
		return nil, err
	}
	c.logger.Debug("jobs discovered", "count", len(jobs), "folder", folderPath)

	var result []jobEntry
	for _, job := range jobs {
		fullPath := job.Name
		if folderPath != "" {
			fullPath = folderPath + "/" + job.Name
		}

		switch job.Class {
		case classWorkflowJob:
			result = append(result, jobEntry{path: fullPath})
		case classFolder:
			subJobs, err := c.discoverJobs(ctx, fullPath)
			if err != nil {
				c.logger.Error("discovering jobs in folder", "folder", fullPath, "error", err)
				continue
			}
			result = append(result, subJobs...)
		case classMultibranchPipeline:
			subJobs, err := c.discoverJobs(ctx, fullPath)
			if err != nil {
				c.logger.Error("discovering jobs in multibranch pipeline", "pipeline", fullPath, "error", err)
				continue
			}
			for _, sub := range subJobs {
				result = append(result, jobEntry{path: sub.path, isBranch: true})
			}
		}
	}
	return result, nil
}

// classifyConfiguredJobs resolves isBranch for each explicitly-configured job
// path by looking up the parent's class via GetJobs. Results from the same
// grandparent folder are cached so sibling branches share one API call.
// On lookup failure the entry falls back to isBranch=false rather than
// aborting the crawl.
func (c *Crawler) classifyConfiguredJobs(ctx context.Context) ([]jobEntry, error) {
	cache := make(map[string][]apiJob)
	var entries []jobEntry
	for _, p := range c.jobs {
		parent := path.Dir(p)
		if parent == "." {
			entries = append(entries, jobEntry{path: p})
			continue
		}
		grandparent := path.Dir(parent)
		if grandparent == "." {
			grandparent = ""
		}
		if _, cached := cache[grandparent]; !cached {
			jobs, err := c.client.GetJobs(ctx, grandparent)
			if err != nil {
				c.logger.Warn("classifying configured job: failed to look up parent class",
					"job", p, "grandparent", grandparent, "error", err)
				cache[grandparent] = nil
			} else {
				cache[grandparent] = jobs
			}
		}
		isBranch := false
		parentName := path.Base(parent)
		for _, sibling := range cache[grandparent] {
			if sibling.Name == parentName && sibling.Class == classMultibranchPipeline {
				isBranch = true
				break
			}
		}
		entries = append(entries, jobEntry{path: p, isBranch: isBranch})
	}
	return entries, nil
}

func (c *Crawler) crawlJob(ctx context.Context, jobPath, workflowID string, builds []apiBuild) error {
	c.logger.Debug("crawling job", "job", jobPath, "workflow", workflowID)
	c.logger.Debug("found builds", "count", len(builds))

	for _, b := range builds {
		buildID := fmt.Sprintf("%s/%d", jobPath, b.Number)
		workflowRun := model.WorkflowRun{
			ID:         buildID,
			WorkflowID: workflowID,
			Number:     b.Number,
			Status:     b.Result,
			Branch:     branchForJob(jobPath, workflowID, b),
			CommitSHA:  extractCommitSHA(b),
			StartedAt:  time.UnixMilli(b.Timestamp),
			Duration:   float64(b.Duration) / 1000.0,
		}
		if err := c.publisher.PublishWorkflowRun(ctx, workflowRun); err != nil {
			c.logger.Error("publishing build as workflow run", "build", buildID, "error", err)
		}

		stages, err := c.client.GetStages(ctx, jobPath, b.Number)
		if err != nil {
			c.logger.Error("fetching stages", "build", buildID, "error", err)
			continue
		}
		c.logger.Debug("found stages", "count", len(stages))

		for i, s := range stages {
			workflowTask := model.WorkflowTask{
				WorkflowRunID: buildID,
				Order:         i,
				Name:          s.Name,
				Status:        s.Status,
				StartedAt:     time.UnixMilli(s.StartTimeMillis),
				Duration:      float64(s.DurationMillis) / 1000.0,
			}
			if err := c.publisher.PublishWorkflowTask(ctx, workflowTask); err != nil {
				c.logger.Error("publishing stage as workflow task", "build", buildID, "stage", s.Name, "error", err)
			}
		}
	}
	return nil
}

// branchForJob returns the branch name for a build. For Multi-branch pipeline
// branch jobs the Git plugin reports all branches that contain the commit
// (commonly "master") rather than the branch being built, so the branch is
// derived from the job path instead. For regular pipeline jobs the Git plugin
// BuildData is authoritative.
func branchForJob(jobPath, workflowID string, b apiBuild) string {
	if path.Dir(jobPath) == workflowID {
		return path.Base(jobPath)
	}
	return extractBranch(b)
}

func extractBranch(b apiBuild) string {
	for _, action := range b.Actions {
		if !strings.Contains(action.Class, "BuildData") {
			continue
		}
		if action.LastBuiltRevision == nil || len(action.LastBuiltRevision.Branch) == 0 {
			continue
		}
		name := action.LastBuiltRevision.Branch[0].Name
		// Strip "origin/" prefix added by the Git plugin
		if after, ok := strings.CutPrefix(name, "origin/"); ok {
			return after
		}
		return name
	}
	return ""
}

func extractCommitSHA(b apiBuild) string {
	for _, action := range b.Actions {
		if !strings.Contains(action.Class, "BuildData") {
			continue
		}
		if action.LastBuiltRevision == nil {
			continue
		}
		return action.LastBuiltRevision.SHA1
	}
	return ""
}

// extractRepoID derives a "projectKey/slug" repo ID from the first remote URL
// found in any build's BuildData action. It takes the last two path segments of
// the URL and strips a trailing ".git" from the slug, e.g.:
//
//	https://bitbucket.example.com/scm/ACME/backend.git → "ACME/backend"
//	git@github.com:org/repo.git                        → "org/repo"
func extractRepoID(builds []apiBuild) string {
	for _, b := range builds {
		for _, action := range b.Actions {
			if !strings.Contains(action.Class, "BuildData") {
				continue
			}
			for _, raw := range action.RemoteUrls {
				if id := repoIDFromURL(raw); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func repoIDFromURL(raw string) string {
	// Normalise SCP-style git URLs (git@host:org/repo.git) to a parseable form.
	if !strings.Contains(raw, "://") {
		raw = "ssh://" + strings.Replace(raw, ":", "/", 1)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return ""
	}
	project := segments[len(segments)-2]
	slug := strings.TrimSuffix(segments[len(segments)-1], ".git")
	if project == "" || slug == "" {
		return ""
	}
	return project + "/" + slug
}
