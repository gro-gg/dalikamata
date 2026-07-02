package component

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
)

// RepoFile is the raw deserialized form of an in-repo self-onboarding config
// file (ADR-007). Unlike ComponentFile it omits the repos list: the repository
// that contains the file is the implied sole member of the component.
type RepoFile struct {
	Version   string `yaml:"version"`
	Team      string `yaml:"team"`
	Component string `yaml:"component"`
}

// ParseRepoFile parses and validates a self-onboarding config file's contents.
func ParseRepoFile(data []byte) (RepoFile, error) {
	var f RepoFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return RepoFile{}, fmt.Errorf("parse repo config: %w", err)
	}
	if err := f.Validate(); err != nil {
		return RepoFile{}, err
	}
	return f, nil
}

// Validate checks schema rules. Returns the first error found. The version is
// validated identically to the central component schema.
func (f RepoFile) Validate() error {
	if f.Version != schemaVersion {
		return fmt.Errorf("unsupported version %q (want %q)", f.Version, schemaVersion)
	}
	if strings.TrimSpace(f.Team) == "" {
		return fmt.Errorf("team is required")
	}
	if strings.TrimSpace(f.Component) == "" {
		return fmt.Errorf("component is required")
	}
	return nil
}

// ToRepoOnboarding maps a validated RepoFile to a domain RepoOnboarding event
// for the given repo. It validates first, so it is safe to call on an
// unvalidated file.
func (f RepoFile) ToRepoOnboarding(repoID string) (model.RepoOnboarding, error) {
	if err := f.Validate(); err != nil {
		return model.RepoOnboarding{}, err
	}
	return model.RepoOnboarding{
		RepoID:    repoID,
		Component: f.Component,
		Team:      f.Team,
	}, nil
}
