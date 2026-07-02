package config_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"codeberg.org/aeforged/dalikamata/internal/ingest/config"
)

// fakePublisher records the teams and components it receives.
type fakePublisher struct {
	teams      []model.Team
	components []model.Component
}

func (f *fakePublisher) PublishTeam(_ context.Context, t model.Team) error {
	f.teams = append(f.teams, t)
	return nil
}

func (f *fakePublisher) PublishComponent(_ context.Context, c model.Component) error {
	f.components = append(f.components, c)
	return nil
}

func (f *fakePublisher) PublishRepoOnboarding(_ context.Context, _ model.RepoOnboarding) error {
	return nil
}

func writeYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const compA = `version: "1"
name: payment-service
team: payments
repos:
  - id: PLAT/payment-service
`

const compB = `version: "1"
name: checkout-api
team: payments
repos:
  - id: PLAT/checkout-api
`

const compC = `version: "1"
name: inventory-sync
team: platform
repos:
  - id: PLAT/inventory-sync
`

func TestCrawler_PublishesAllEntities(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "a.yaml", compA)
	writeYAML(t, dir, "b.yaml", compB)
	writeYAML(t, dir, "c.yaml", compC)

	pub := &fakePublisher{}
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	crawler := config.NewCrawler(pub, dir, l)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(pub.components) != 3 {
		t.Errorf("components published = %d, want 3", len(pub.components))
	}
}

func TestCrawler_DeduplicatesTeams(t *testing.T) {
	dir := t.TempDir()
	// compA and compB share team "payments", compC has team "platform"
	writeYAML(t, dir, "a.yaml", compA)
	writeYAML(t, dir, "b.yaml", compB)
	writeYAML(t, dir, "c.yaml", compC)

	pub := &fakePublisher{}
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	crawler := config.NewCrawler(pub, dir, l)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// "payments" appears twice in files but must be published only once
	if len(pub.teams) != 2 {
		t.Errorf("teams published = %d, want 2 (payments + platform)", len(pub.teams))
	}
}

func TestCrawler_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	pub := &fakePublisher{}
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	crawler := config.NewCrawler(pub, dir, l)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("Run on empty dir: %v", err)
	}
	if len(pub.teams) != 0 || len(pub.components) != 0 {
		t.Errorf("expected nothing published, got teams=%d components=%d", len(pub.teams), len(pub.components))
	}
}

func TestCrawler_RepoIDsPreserved(t *testing.T) {
	const yaml = `version: "1"
name: svc
team: alpha
repos:
  - id: PROJ/svc-api
  - id: PROJ/svc-worker
`
	dir := t.TempDir()
	writeYAML(t, dir, "svc.yaml", yaml)

	pub := &fakePublisher{}
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	crawler := config.NewCrawler(pub, dir, l)

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(pub.components) != 1 {
		t.Fatalf("components = %d, want 1", len(pub.components))
	}
	if len(pub.components[0].RepoIDs) != 2 {
		t.Fatalf("repo_ids len = %d, want 2", len(pub.components[0].RepoIDs))
	}
	if pub.components[0].RepoIDs[0] != "PROJ/svc-api" {
		t.Errorf("repo_ids[0] = %q, want PROJ/svc-api", pub.components[0].RepoIDs[0])
	}
}

func TestCrawler_InvalidFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "bad.yaml", "version: \"2\"\nname: x\nteam: t\nrepos: []\n")

	pub := &fakePublisher{}
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	crawler := config.NewCrawler(pub, dir, l)

	if err := crawler.Run(context.Background()); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
