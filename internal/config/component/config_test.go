package component_test

import (
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/aeforged/dalikamata/internal/config/component"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const goldenYAML = `version: "1"
name: payment-service
team: payments
repos:
  - id: PLAT/payment-service
    role: cicd
  - id: PLAT/payment-infra
    role: cd
workflows:
  - id: payment-service-build
    role: ci
  - id: payment-service-deploy
    role: cd
artifacts:
  - name: payment-api
    type: docker-image
    repository: registry.example.com/payment-api
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
	if f.Repos[0].ID != "PLAT/payment-service" || f.Repos[0].Role != "cicd" {
		t.Errorf("repos[0] = %+v", f.Repos[0])
	}
	if len(f.Workflows) != 2 {
		t.Fatalf("workflows len = %d, want 2", len(f.Workflows))
	}
	if len(f.Artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(f.Artifacts))
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
			yaml:    "version: \"2\"\nname: x\nteam: t\nrepos:\n  - id: r\n    role: ci\nworkflows:\n  - id: w\n    role: ci\n",
			wantErr: "unsupported version",
		},
		{
			name:    "missing name",
			yaml:    "version: \"1\"\nname: \"\"\nteam: t\nrepos:\n  - id: r\n    role: ci\nworkflows:\n  - id: w\n    role: ci\n",
			wantErr: "name is required",
		},
		{
			name:    "missing team",
			yaml:    "version: \"1\"\nname: x\nteam: \"\"\nrepos:\n  - id: r\n    role: ci\nworkflows:\n  - id: w\n    role: ci\n",
			wantErr: "team is required",
		},
		{
			name:    "empty repos",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos: []\nworkflows:\n  - id: w\n    role: ci\n",
			wantErr: "repos must not be empty",
		},
		{
			name:    "unknown repo role",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos:\n  - id: r\n    role: both\nworkflows:\n  - id: w\n    role: ci\n",
			wantErr: `repos[0].role "both"`,
		},
		{
			name:    "duplicate repo id",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos:\n  - id: r\n    role: ci\n  - id: r\n    role: cd\nworkflows:\n  - id: w\n    role: ci\n",
			wantErr: `repos[1].id "r" is duplicated`,
		},
		{
			name:    "empty workflows",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos:\n  - id: r\n    role: ci\nworkflows: []\n",
			wantErr: "workflows must not be empty",
		},
		{
			name:    "unknown workflow role",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos:\n  - id: r\n    role: ci\nworkflows:\n  - id: w\n    role: both\n",
			wantErr: `workflows[0].role "both"`,
		},
		{
			name:    "duplicate workflow id",
			yaml:    "version: \"1\"\nname: x\nteam: t\nrepos:\n  - id: r\n    role: ci\nworkflows:\n  - id: w\n    role: ci\n  - id: w\n    role: cd\n",
			wantErr: `workflows[1].id "w" is duplicated`,
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

	_, err := component.LoadDir(dir)
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
    role: cicd
workflows:
  - id: checkout-build
    role: ci
`
	dir := t.TempDir()
	writeFile(t, dir, "payment-service.yaml", goldenYAML)
	writeFile(t, dir, "checkout-api.yaml", secondYAML)

	files, err := component.LoadDir(dir)
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
	if len(comp.Repos) != 2 {
		t.Fatalf("repos len = %d, want 2", len(comp.Repos))
	}
	if comp.Repos[0].Role != model.DeliveryRoleCICD {
		t.Errorf("repos[0].Role = %q, want CICD", comp.Repos[0].Role)
	}
	if comp.Repos[1].Role != model.DeliveryRoleCD {
		t.Errorf("repos[1].Role = %q, want CD", comp.Repos[1].Role)
	}
	if len(comp.Workflows) != 2 {
		t.Fatalf("workflows len = %d, want 2", len(comp.Workflows))
	}
	if comp.Workflows[0].Role != model.DeliveryRoleCI {
		t.Errorf("workflows[0].Role = %q, want CI", comp.Workflows[0].Role)
	}
	if len(comp.Artifacts) != 1 {
		t.Errorf("artifacts len = %d, want 1", len(comp.Artifacts))
	}
}

func TestConvertToDomain_RoleCaseNormalization(t *testing.T) {
	// roles in YAML may be uppercase or mixed-case; convert normalises them
	const mixedCaseYAML = `version: "1"
name: svc
team: team-a
repos:
  - id: r
    role: CICD
workflows:
  - id: w
    role: CI
`
	dir := t.TempDir()
	p := writeFile(t, dir, "svc.yaml", mixedCaseYAML)
	f, err := component.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, comp, err := component.ConvertToDomain(f)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if comp.Repos[0].Role != model.DeliveryRoleCICD {
		t.Errorf("role = %q, want CICD", comp.Repos[0].Role)
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
