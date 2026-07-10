package component

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const schemaVersion = "1"

// ComponentFile is the raw deserialized form of a component YAML file.
// It maps 1:1 to the on-disk schema; no domain semantics.
type ComponentFile struct {
	Version string    `yaml:"version"`
	Name    string    `yaml:"name"`
	Team    string    `yaml:"team"`
	Repos   []RepoRef `yaml:"repos"`
}

type RepoRef struct {
	ID string `yaml:"id"`
}

// Load reads and validates a single component YAML file.
func Load(path string) (ComponentFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-supplied config path by design
	if err != nil {
		return ComponentFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	var f ComponentFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return ComponentFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := f.Validate(); err != nil {
		return ComponentFile{}, fmt.Errorf("validate %s: %w", path, err)
	}
	return f, nil
}

// LoadDir reads all *.yaml and *.yml files in dir, returning them in
// filename-sorted order. Duplicate component names across files are rejected.
// It is fail-soft on individual files: a file that fails to load (read, parse,
// or validate) is logged and skipped, never aborting the rest of the directory.
func LoadDir(dir string, logger *slog.Logger) ([]ComponentFile, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	ymlEntries, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	entries = append(entries, ymlEntries...)

	seen := make(map[string]string) // name -> filename
	var files []ComponentFile
	for _, path := range entries {
		f, err := Load(path)
		if err != nil {
			logger.Error("invalid component config; skipping file", "path", path, "error", err)
			continue
		}
		if prev, ok := seen[f.Name]; ok {
			return nil, fmt.Errorf("duplicate component name %q in %s (already defined in %s)", f.Name, filepath.Base(path), filepath.Base(prev))
		}
		seen[f.Name] = path
		files = append(files, f)
	}
	return files, nil
}

// Validate checks schema rules. Returns the first error found.
func (f ComponentFile) Validate() error {
	if f.Version != schemaVersion {
		return fmt.Errorf("unsupported version %q (want %q)", f.Version, schemaVersion)
	}
	if strings.TrimSpace(f.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(f.Team) == "" {
		return fmt.Errorf("team is required")
	}
	if len(f.Repos) == 0 {
		return fmt.Errorf("repos must not be empty")
	}
	seenRepo := make(map[string]bool)
	for i, r := range f.Repos {
		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("repos[%d].id is required", i)
		}
		if seenRepo[r.ID] {
			return fmt.Errorf("repos[%d].id %q is duplicated", i, r.ID)
		}
		seenRepo[r.ID] = true
	}
	return nil
}
