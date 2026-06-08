package bitbucket

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// Crawler performs a full crawl of the configured Bitbucket projects.
type Crawler struct {
	client    BitbucketClient
	publisher domain.GitPublisher
	projects  []string
	logger    *slog.Logger
}

func NewCrawler(client BitbucketClient, publisher domain.GitPublisher, projects []string, logger *slog.Logger) *Crawler {
	return &Crawler{
		client:    client,
		publisher: publisher,
		projects:  projects,
		logger:    logger.With("component", "bitbucket_crawler"),
	}
}

func (c *Crawler) Crawl(ctx context.Context) error {
	c.logger.Info("Start Crawling Bitbucket")

	for _, projectKey := range c.projects {
		if err := c.crawlProject(ctx, projectKey); err != nil {
			c.logger.Error("crawling project", "project", projectKey, "error", err)
		}
	}

	c.logger.Info("Finished Crawling Bitbucket")
	return nil
}

func (c *Crawler) crawlProject(ctx context.Context, projectKey string) error {
	c.logger.Info("crawling project", "project", projectKey)

	repos, err := c.client.GetRepos(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("get repos for project %s: %w", projectKey, err)
	}

	for _, apiRepo := range repos {
		repo := model.Repo{
			RepoID: model.NewRepoID(projectKey, apiRepo.Slug),
			Name:   apiRepo.Name,
		}
		err = c.publisher.PublishRepo(ctx, repo)
		if err != nil {
			c.logger.Error("publish repo", "project", projectKey, "repo", apiRepo.Slug, "error", err)
		}
		if err = c.crawlRepo(ctx, projectKey, apiRepo.Slug); err != nil {
			c.logger.Error("crawl repo", "project", projectKey, "repo", apiRepo.Slug, "error", err)
		}
	}
	return nil
}

func (c *Crawler) crawlRepo(ctx context.Context, projectKey, repoSlug string) error {
	c.logger.Info("crawling repo", "project", projectKey, "repo", repoSlug)

	if err := c.crawlCommits(ctx, projectKey, repoSlug); err != nil {
		return fmt.Errorf("crawl commits: %w", err)
	}

	if err := c.crawlPullRequests(ctx, projectKey, repoSlug); err != nil {
		return fmt.Errorf("crawl pull requests: %w", err)
	}

	return nil
}

func (c *Crawler) crawlCommits(ctx context.Context, projectKey, repoSlug string) error {
	commits, err := c.client.GetCommits(ctx, projectKey, repoSlug, "")
	if err != nil {
		return fmt.Errorf("get commits: %w", err)
	}

	for _, commit := range commits {
		event := model.Commit{
			SHA:       commit.ID,
			RepoID:    model.NewRepoID(projectKey, repoSlug),
			Author:    commit.Author.Name,
			Timestamp: time.UnixMilli(commit.AuthorTimestamp),
		}
		c.logger.Debug("found commit", "repo", repoSlug, "sha", event.SHA, "author", event.Author, "timestamp", event.Timestamp)
		if err := c.publisher.PublishCommit(ctx, event); err != nil {
			c.logger.Error("publish commit", "sha", event.SHA, "error", err)
		}
	}
	return nil
}

func (c *Crawler) crawlPullRequests(ctx context.Context, projectKey, repoSlug string) error {
	prs, err := c.client.GetPullRequests(ctx, projectKey, repoSlug)
	if err != nil {
		return fmt.Errorf("get pull requests: %w", err)
	}

	for _, pr := range prs {
		event := model.PullRequest{
			ID:          model.NewPullRequestID(projectKey, repoSlug, fmt.Sprintf("%d", pr.ID)),
			RepoID:      model.NewRepoID(projectKey, repoSlug),
			Name:        fmt.Sprintf("%d", pr.ID),
			Title:       pr.Title,
			Description: pr.Description,
			State:       pr.State,
			Author:      pr.Author.User.DisplayName,
			CreatedAt:   time.UnixMilli(pr.CreatedDate),
			UpdatedAt:   time.UnixMilli(pr.UpdatedDate),
		}
		c.logger.Debug("found pull request", "repo", repoSlug, "id", event.ID, "title", event.Title, "state", event.State, "author", event.Author)
		if err := c.publisher.PublishPullRequest(ctx, event); err != nil {
			c.logger.Error("publish pull request", "id", event.ID, "error", err)
		}
	}
	return nil
}
