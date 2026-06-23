package component

import (
	"strings"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

var roleMap = map[string]string{
	"ci":   model.DeliveryRoleCI,
	"cd":   model.DeliveryRoleCD,
	"cicd": model.DeliveryRoleCICD,
}

// ConvertToDomain maps a validated ComponentFile to its domain representations.
// It validates first; callers that have already called Validate may ignore the
// error, but it is safe to call on an unvalidated file.
func ConvertToDomain(f ComponentFile) (model.Team, model.Component, error) {
	if err := f.Validate(); err != nil {
		return model.Team{}, model.Component{}, err
	}

	team := model.Team{Name: f.Team}

	repos := make([]model.ComponentRepo, len(f.Repos))
	for i, r := range f.Repos {
		repos[i] = model.ComponentRepo{
			RepoID: r.ID,
			Role:   roleMap[strings.ToLower(r.Role)],
		}
	}

	workflows := make([]model.ComponentWorkflow, len(f.Workflows))
	for i, w := range f.Workflows {
		workflows[i] = model.ComponentWorkflow{
			WorkflowID: w.ID,
			Role:       roleMap[strings.ToLower(w.Role)],
		}
	}

	comp := model.Component{
		Name:      f.Name,
		TeamName:  f.Team,
		Repos:     repos,
		Workflows: workflows,
	}
	return team, comp, nil
}
