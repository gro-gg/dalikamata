package model

type Team struct {
	Name string `json:"name"`
}

type Component struct {
	Name     string   `json:"name"`
	TeamName string   `json:"team_name"`
	RepoIDs  []string `json:"repo_ids"`
}

// OwnershipDiagnostics reports how a single Workflow resolves to a team via
// the Workflow.RepoID → Component.RepoIDs → Component.TeamName chain. Reason
// is one of "ok", "missing_repo_id", "no_component_for_repo", or
// "no_team_for_component".
type OwnershipDiagnostics struct {
	WorkflowID    string `json:"workflow_id"`
	RepoID        string `json:"repo_id"`
	ComponentName string `json:"component_name"`
	TeamName      string `json:"team_name"`
	Reason        string `json:"reason"`
}
