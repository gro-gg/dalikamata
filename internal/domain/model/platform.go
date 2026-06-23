package model

type Team struct {
	Name string `json:"name"`
}

type Component struct {
	Name     string   `json:"name"`
	TeamName string   `json:"team_name"`
	RepoIDs  []string `json:"repo_ids"`
}
