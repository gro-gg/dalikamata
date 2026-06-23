package component

import (
	"codeberg.org/aeforged/dalikamata/internal/domain/model"
)

// ConvertToDomain maps a validated ComponentFile to its domain representations.
// It validates first; callers that have already called Validate may ignore the
// error, but it is safe to call on an unvalidated file.
func ConvertToDomain(f ComponentFile) (model.Team, model.Component, error) {
	if err := f.Validate(); err != nil {
		return model.Team{}, model.Component{}, err
	}

	team := model.Team{Name: f.Team}

	repoIDs := make([]string, len(f.Repos))
	for i, r := range f.Repos {
		repoIDs[i] = r.ID
	}

	comp := model.Component{
		Name:     f.Name,
		TeamName: f.Team,
		RepoIDs:  repoIDs,
	}
	return team, comp, nil
}
