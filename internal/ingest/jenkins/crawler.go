package jenkins

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const (
	classWorkflowJob         = "org.jenkinsci.plugins.workflow.job.WorkflowJob"
	classFolder              = "com.cloudbees.hudson.plugins.folder.Folder"
	classMultibranchPipeline = "org.jenkinsci.plugins.workflow.multibranch.WorkflowMultiBranchProject"
	classGitBuildData        = "hudson.plugins.git.util.BuildData"
)

type Crawler struct {
	client    JenkinsClient
	publisher domain.CICDPublisher
	jobs      []string
	logger    *slog.Logger
}

func NewCrawler(client JenkinsClient, publisher domain.CICDPublisher, jobs []string, logger *slog.Logger) *Crawler {
	return &Crawler{
		client:    client,
		publisher: publisher,
		jobs:      jobs,
		logger:    logger.With("component", "jenkins-crawler"),
	}
}

func (c *Crawler) Crawl(ctx context.Context) error {
	jobPaths := c.jobs
	if len(jobPaths) == 0 {
		c.logger.Info("No jobs specified, discovering all jobs")
		var err error
		jobPaths, err = c.discoverJobs(ctx, "")
		if err != nil {
			return fmt.Errorf("discovering jobs: %w", err)
		}
		c.logger.Info("Discovered jobs", "count", len(jobPaths), "jobs", jobPaths)
	}

	for _, jobPath := range jobPaths {
		if err := c.crawlJob(ctx, jobPath); err != nil {
			c.logger.Error("crawling job", "job", jobPath, "error", err)
		}
	}
	return nil
}

func (c *Crawler) discoverJobs(ctx context.Context, folderPath string) ([]string, error) {
	jobs, err := c.client.GetJobs(ctx, folderPath)
	if err != nil {
		return nil, err
	}
	c.logger.Debug("jobs discovered", "count", len(jobs), "folder", folderPath)

	var result []string
	for _, job := range jobs {
		fullPath := job.Name
		if folderPath != "" {
			fullPath = folderPath + "/" + job.Name
		}

		switch job.Class {
		case classWorkflowJob:
			result = append(result, fullPath)
		case classFolder, classMultibranchPipeline:
			subJobs, err := c.discoverJobs(ctx, fullPath)
			if err != nil {
				c.logger.Error("discovering jobs in folder", "folder", fullPath, "error", err)
				continue
			}
			result = append(result, subJobs...)
		}
	}
	return result, nil
}

func (c *Crawler) crawlJob(ctx context.Context, jobPath string) error {
	c.logger.Debug("crawling job", "job", jobPath)

	workflow := model.Workflow{
		ID:   jobPath,
		Name: path.Base(jobPath),
	}
	if err := c.publisher.PublishWorkflow(ctx, workflow); err != nil {
		c.logger.Error("publishing job as workflow", "job", jobPath, "error", err)
	}

	builds, err := c.client.GetBuilds(ctx, jobPath)
	if err != nil {
		return fmt.Errorf("fetching builds: %w", err)
	}
	c.logger.Debug("found builds", "count", len(builds))

	for _, b := range builds {
		if b.InProgress || b.Result == "" {
			continue
		}

		buildID := fmt.Sprintf("%s/%d", jobPath, b.Number)
		workflowRun := model.WorkflowRun{
			ID:         buildID,
			WorkflowID: jobPath,
			Number:     b.Number,
			Status:     b.Result,
			Branch:     extractBranch(b),
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
