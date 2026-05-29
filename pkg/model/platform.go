package model

const (
	DeliveryRoleCI   = "CI"
	DeliveryRoleCD   = "CD"
	DeliveryRoleCICD = "CICD"
)

type Team struct {
	Name string `json:"name"`
}

type ComponentRepo struct {
	RepoID string `json:"repo_id"`
	Role   string `json:"role"`
}

type ComponentWorkflow struct {
	WorkflowID string `json:"workflow_id"`
	Role       string `json:"role"`
}

type Artifact struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Repository string `json:"repository"`
}

type Component struct {
	Name      string              `json:"name"`
	TeamName  string              `json:"team_name"`
	Repos     []ComponentRepo     `json:"repos"`
	Workflows []ComponentWorkflow `json:"workflows"`
	Artifacts []Artifact          `json:"artifacts"`
}
