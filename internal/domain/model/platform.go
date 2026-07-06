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

// OwnershipDiagnostics reports how a single Workflow resolves to a team via
// the Workflow.RepoIDs → Component.RepoIDs → Component.TeamName chain. A
// workflow may reference several repos (e.g. app repo plus shared libraries);
// ownership resolves to the first repo that maps to a known component. Reason
// is one of "ok", "missing_repo_id" (the workflow has no repos),
// "no_component_for_repo" (none of the repos map to a component), or
// "no_team_for_component".
type OwnershipDiagnostics struct {
	WorkflowID    string   `json:"workflow_id"`
	RepoIDs       []string `json:"repo_ids"`
	ComponentName string   `json:"component_name"`
	TeamName      string   `json:"team_name"`
	Reason        string   `json:"reason"`
}
