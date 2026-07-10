package component_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/aeforged/dalikamata/internal/config/component"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const goldenYAML = `version: "1"
name: payment-service
team: payments
repos:
  - id: PLAT/payment-service
  - id: PLAT/payment-infra
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_Golden(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "payment-service.yaml", goldenYAML)

	f, err := component.Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Name != "payment-service" {
		t.Errorf("name = %q, want payment-service", f.Name)
	}
	if f.Team != "payments" {
		t.Errorf("team = %q, want payments", f.Team)
	}
	if len(f.Repos) != 2 {
		t.Fatalf("repos len = %d, want 2", len(f.Repos))
	}
	if f.Repos[0].ID != "PLAT/payment-service" {
		t.Errorf("repos[0] = %+v", f.Repos[0])
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "unknown version",
			yaml:    "version: \"2\"\nname: x\nteam: t\nrepos:\n  - id: r\n",
			wantErr: "unsupported version",
		},
		{
			name:    "missing name",
			yaml:    "version: \"1\"\nname: \"\"\nteam: t\nrepos:\n  - id: r\n",
			wantErr: "name is required",
		},
		{
			name:    "missing team",
			yaml:    "version: \"1\"\nname: x\nteam: \"\"\nrepos:\n  - id: r\n",
			wantErr: "team is required",
		},
		{
			name:    "empty repos",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos: []\n",
			wantErr: "repos must not be empty",
		},
		{
			name:    "duplicate repo id",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos:\n  - id: r\n  - id: r\n",
			wantErr: `repos[1].id "r" is duplicated`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := writeFile(t, dir, "comp.yaml", tc.yaml)
			_, err := component.Load(p)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.wantErr != "" && !contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadDir_Dedup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", goldenYAML)
	writeFile(t, dir, "b.yaml", goldenYAML) // same name

	_, err := component.LoadDir(dir, discardLogger())
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
	if !contains(err.Error(), "duplicate component name") {
		t.Errorf("error = %q, want duplicate component name", err.Error())
	}
}

func TestLoadDir_MultipleFiles(t *testing.T) {
	const secondYAML = `version: "1"
name: checkout-api
team: payments
repos:
  - id: PLAT/checkout-api
`
	dir := t.TempDir()
	writeFile(t, dir, "payment-service.yaml", goldenYAML)
	writeFile(t, dir, "checkout-api.yaml", secondYAML)

	files, err := component.LoadDir(dir, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
}

func TestLoadDir_SkipsInvalidFile(t *testing.T) {
	const secondYAML = `version: "1"
name: checkout-api
team: payments
repos:
  - id: PLAT/checkout-api
`
	dir := t.TempDir()
	writeFile(t, dir, "payment-service.yaml", goldenYAML)
	writeFile(t, dir, "checkout-api.yaml", secondYAML)
	writeFile(t, dir, "broken.yaml", "version: \"1\"\nname: \"\"\nteam: t\nrepos:\n  - id: r\n")

	files, err := component.LoadDir(dir, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
}

func TestConvertToDomain(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "payment-service.yaml", goldenYAML)

	f, err := component.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	team, comp, err := component.ConvertToDomain(f)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if team.Name != "payments" {
		t.Errorf("team.Name = %q, want payments", team.Name)
	}
	if comp.Name != "payment-service" {
		t.Errorf("comp.Name = %q, want payment-service", comp.Name)
	}
	if comp.TeamName != "payments" {
		t.Errorf("comp.TeamName = %q, want payments", comp.TeamName)
	}
	if len(comp.RepoIDs) != 2 {
		t.Fatalf("repo_ids len = %d, want 2", len(comp.RepoIDs))
	}
	if comp.RepoIDs[0] != "PLAT/payment-service" {
		t.Errorf("repo_ids[0] = %q, want PLAT/payment-service", comp.RepoIDs[0])
	}
	if comp.RepoIDs[1] != "PLAT/payment-infra" {
		t.Errorf("repo_ids[1] = %q, want PLAT/payment-infra", comp.RepoIDs[1])
	}
}

func TestConvertToDomain_MultipleRepos(t *testing.T) {
	const multiRepoYAML = `version: "1"
name: svc
team: team-a
repos:
  - id: r1
  - id: r2
`
	dir := t.TempDir()
	p := writeFile(t, dir, "svc.yaml", multiRepoYAML)
	f, err := component.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, comp, err := component.ConvertToDomain(f)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(comp.RepoIDs) != 2 || comp.RepoIDs[0] != "r1" || comp.RepoIDs[1] != "r2" {
		t.Errorf("repo_ids = %v, want [r1 r2]", comp.RepoIDs)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub)))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
