package model

type Team struct {
	Name string `json:"name"`
}

type Component struct {
	Name     string   `json:"name"`
	TeamName string   `json:"team_name"`
	RepoIDs  []string `json:"repo_ids"`
}

// RepoOnboarding is a per-repo self-onboarding declaration (ADR-007): the repo
// RepoID belongs to Component, owned by Team. Handling it upserts the Team and
// the Component and reassigns the repo to that component, removing it from any
// other component it previously belonged to. Unlike the central config crawler
// it carries a single repo rather than a whole component's repo list.
type RepoOnboarding struct {
	RepoID    string `json:"repo_id"`
	Component string `json:"component"`
	Team      string `json:"team"`
}

// RepoOwnership is the resolution result for a single repo of a Workflow's
// RepoIDs, one arm of the Workflow.RepoIDs → Component.RepoIDs →
// Component.TeamName chain. Reason is "ok" (ComponentName/TeamName set),
// "no_component_for_repo" (the repo maps to no component), or
// "no_team_for_component" (the component has no team).
type RepoOwnership struct {
	RepoID        string `json:"repo_id"`
	ComponentName string `json:"component_name,omitempty"`
	TeamName      string `json:"team_name,omitempty"`
	Reason        string `json:"reason"`
}

// OwnershipDiagnostics reports how a single Workflow resolves to its owners
// via the Workflow.RepoIDs → Component.RepoIDs → Component.TeamName chain. A
// workflow may reference several repos (e.g. app repo plus shared libraries)
// belonging to different components, so ownership is not a single pair: Owners
// lists the resolution outcome for every one of the workflow's repos. The
// top-level Reason is "ok" if at least one repo fully resolves to a team,
// "missing_repo_id" if the workflow has no repos, "no_team_for_component" if
// at least one repo reaches a component but none reach a team, or
// "no_component_for_repo" if no repo maps to any component.
type OwnershipDiagnostics struct {
	WorkflowID string          `json:"workflow_id"`
	Reason     string          `json:"reason"`
	Owners     []RepoOwnership `json:"owners,omitempty"`
}
