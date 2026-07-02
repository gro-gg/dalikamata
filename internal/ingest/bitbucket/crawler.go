package bitbucket

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/config/component"
	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/domain/model"
)

// RepoOnboardingPublisher is the outgoing port the crawler uses to publish
// per-repo self-onboarding events (ADR-007). It is optional: when nil, the
// crawler never fetches in-repo config files and behaves exactly as before.
type RepoOnboardingPublisher interface {
	PublishRepoOnboarding(ctx context.Context, o model.RepoOnboarding) error
}

// Crawler performs incremental crawls of the configured Bitbucket projects.
type Crawler struct {
	client    BitbucketClient
	publisher domain.GitPublisher
	store     Cursors
	projects  []string
	logger    *slog.Logger

	// Self-onboarding (ADR-007). Both are set together via WithComponentConfig;
	// when configPublisher is nil, self-onboarding is disabled.
	configPublisher RepoOnboardingPublisher
	configPath      string

	mu      sync.Mutex
	cursors map[string]string // repoID → newest published SHA
	loaded  bool              // true after the first Load from the store
}

// CrawlerOption configures optional Crawler behaviour.
type CrawlerOption func(*Crawler)

// WithComponentConfig enables per-repo self-onboarding: for each crawled repo
// the crawler fetches path from the repo root and, when present, publishes a
// RepoOnboarding event via publisher.
func WithComponentConfig(publisher RepoOnboardingPublisher, path string) CrawlerOption {
	return func(c *Crawler) {
		c.configPublisher = publisher
		c.configPath = path
	}
}

func NewCrawler(client BitbucketClient, publisher domain.GitPublisher, store Cursors, projects []string, logger *slog.Logger, opts ...CrawlerOption) *Crawler {
	c := &Crawler{
		client:    client,
		publisher: publisher,
		store:     store,
		projects:  projects,
		cursors:   map[string]string{},
		logger:    logger.With("component", "bitbucket_crawler"),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Crawler) Crawl(ctx context.Context) error {
	c.logger.Info("Start Crawling Bitbucket")

	// Hydrate per-repo cursors from the persistent store on the very first tick.
	// On failure we log and continue with an empty map (full refetch), which is
	// safe because downstream storage is idempotent.
	c.mu.Lock()
	if !c.loaded {
		c.mu.Unlock()
		loaded, err := c.store.Load(ctx)
		c.mu.Lock()
		if err != nil {
			c.logger.Warn("loading commit cursors failed; starting fresh", "error", err)
			loaded = map[string]string{}
		}
		c.cursors = loaded
		c.loaded = true
	}
	c.mu.Unlock()

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
		c.selfOnboardRepo(ctx, projectKey, apiRepo.Slug, repo.RepoID)
		if err = c.crawlRepo(ctx, projectKey, apiRepo.Slug); err != nil {
			c.logger.Error("crawl repo", "project", projectKey, "repo", apiRepo.Slug, "error", err)
		}
	}
	return nil
}

// selfOnboardRepo fetches the in-repo self-onboarding config for one repo and,
// when present and valid, publishes a RepoOnboarding event. It is fail-soft: a
// missing, unfetchable, or invalid file is logged and skipped so it never
// aborts the crawl. A no-op when self-onboarding is disabled.
func (c *Crawler) selfOnboardRepo(ctx context.Context, projectKey, repoSlug, repoID string) {
	if c.configPublisher == nil || c.configPath == "" {
		return
	}

	data, found, err := c.client.GetRawFile(ctx, projectKey, repoSlug, c.configPath)
	if err != nil {
		c.logger.Warn("fetching component config; skipping self-onboarding",
			"project", projectKey, "repo", repoSlug, "path", c.configPath, "error", err)
		return
	}
	if !found {
		c.logger.Debug("no component config; repo not self-onboarded",
			"project", projectKey, "repo", repoSlug, "path", c.configPath)
		return
	}

	f, err := component.ParseRepoFile(data)
	if err != nil {
		c.logger.Warn("invalid component config; skipping self-onboarding",
			"project", projectKey, "repo", repoSlug, "path", c.configPath, "error", err)
		return
	}

	onboarding, err := f.ToRepoOnboarding(repoID)
	if err != nil {
		c.logger.Warn("converting component config; skipping self-onboarding",
			"project", projectKey, "repo", repoSlug, "error", err)
		return
	}

	c.logger.Info("repo self-onboarded", "repo", repoID, "component", onboarding.Component, "team", onboarding.Team)
	if err := c.configPublisher.PublishRepoOnboarding(ctx, onboarding); err != nil {
		c.logger.Error("publish repo onboarding", "repo", repoID, "error", err)
	}
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
	repoID := model.NewRepoID(projectKey, repoSlug)

	c.mu.Lock()
	since := c.cursors[repoID]
	c.mu.Unlock()

	commits, err := c.client.GetCommits(ctx, projectKey, repoSlug, since)
	if err != nil && since != "" {
		// The cursor SHA may have been rewritten (force-push). Clear it and
		// fall back to a full refetch for this repo; the duplicate commits are
		// collapsed downstream by the idempotent in-memory repository.
		c.logger.Warn("GetCommits with cursor failed; retrying from scratch",
			"repo", repoID, "since", since, "error", err)
		if clearErr := c.store.Clear(ctx, repoID); clearErr != nil {
			c.logger.Error("clearing cursor after GetCommits error", "repo", repoID, "error", clearErr)
		}
		c.mu.Lock()
		delete(c.cursors, repoID)
		c.mu.Unlock()
		commits, err = c.client.GetCommits(ctx, projectKey, repoSlug, "")
	}
	if err != nil {
		return fmt.Errorf("get commits: %w", err)
	}
	if len(commits) == 0 {
		return nil
	}

	for _, commit := range commits {
		event := model.Commit{
			SHA:       commit.ID,
			RepoID:    repoID,
			Author:    commit.Author.Name,
			Timestamp: time.UnixMilli(commit.AuthorTimestamp),
		}
		c.logger.Debug("found commit", "repo", repoSlug, "sha", event.SHA, "author", event.Author, "timestamp", event.Timestamp)
		if err := c.publisher.PublishCommit(ctx, event); err != nil {
			c.logger.Error("publish commit", "sha", event.SHA, "error", err)
		}
	}

	// commits[0] is the newest commit (Bitbucket returns reverse-chronological order).
	// Save the cursor only after publishing to preserve "side of duplicate pulls":
	// a failed save means the next tick re-fetches, not skips.
	newSHA := commits[0].ID
	if saveErr := c.store.Save(ctx, repoID, newSHA); saveErr != nil {
		c.logger.Error("saving commit cursor", "repo", repoID, "sha", newSHA, "error", saveErr)
		return nil
	}
	c.mu.Lock()
	c.cursors[repoID] = newSHA
	c.mu.Unlock()

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
