package config

import (
	"context"
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/config/component"
	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// Crawler reads component YAML files from a directory and publishes the
// resulting Team and Component events. It is a one-shot crawler: each Run call
// processes all files in the directory once. Re-runs are idempotent because
// the domain repository upserts entities by name.
type Crawler struct {
	publisher domain.PlatformPublisher
	dir       string
	logger    *slog.Logger
}

func NewCrawler(publisher domain.PlatformPublisher, dir string, logger *slog.Logger) *Crawler {
	return &Crawler{
		publisher: publisher,
		dir:       dir,
		logger:    logger.With("component", "config-crawler"),
	}
}

// Run loads all component YAML files in the configured directory, converts them
// to domain objects, deduplicates teams, and publishes all events.
func (c *Crawler) Run(ctx context.Context) error {
	files, err := component.LoadDir(c.dir)
	if err != nil {
		return fmt.Errorf("loading component configs from %s: %w", c.dir, err)
	}
	c.logger.Info("loaded component configs", "count", len(files), "dir", c.dir)

	seen := make(map[string]bool)
	for _, f := range files {
		team, comp, err := component.ConvertToDomain(f)
		if err != nil {
			return fmt.Errorf("converting %s: %w", f.Name, err)
		}

		if !seen[team.Name] {
			if err := c.publishTeam(ctx, team); err != nil {
				return err
			}
			seen[team.Name] = true
		}

		if err := c.publishComponent(ctx, comp); err != nil {
			return err
		}
	}
	return nil
}

func (c *Crawler) publishTeam(ctx context.Context, team model.Team) error {
	c.logger.Debug("publishing team", "name", team.Name)
	if err := c.publisher.PublishTeam(ctx, team); err != nil {
		return fmt.Errorf("publishing team %s: %w", team.Name, err)
	}
	return nil
}

func (c *Crawler) publishComponent(ctx context.Context, comp model.Component) error {
	c.logger.Debug("publishing component", "name", comp.Name, "team", comp.TeamName)
	if err := c.publisher.PublishComponent(ctx, comp); err != nil {
		return fmt.Errorf("publishing component %s: %w", comp.Name, err)
	}
	return nil
}
