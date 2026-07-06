package repo_test

import (
	"context"
	"sort"
	"testing"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

// componentsByName snapshots all components keyed by name via QueryComponents.
func componentsByName(t *testing.T, r *repo.MemoryRepository) map[string]model.Component {
	t.Helper()
	out := make(map[string]model.Component)
	err := r.QueryComponents(context.Background(), query.Query{Entity: query.EntityComponent}, func(c model.Component) error {
		out[c.Name] = c
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func teamNames(t *testing.T, r *repo.MemoryRepository) map[string]bool {
	t.Helper()
	out := make(map[string]bool)
	err := r.QueryTeams(context.Background(), query.Query{Entity: query.EntityTeam}, func(tm model.Team) error {
		out[tm.Name] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAddRepoOnboarding_UpsertsTeamAndComponent verifies a single onboarding
// event creates the team and a component containing the repo.
func TestAddRepoOnboarding_UpsertsTeamAndComponent(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	err := r.AddRepoOnboarding(ctx, model.RepoOnboarding{RepoID: "PROJ/backend-api", Component: "backend", Team: "platform"})
	is.NoErr(err)

	is.True(teamNames(t, r)["platform"])

	comps := componentsByName(t, r)
	is.Equal(len(comps), 1)
	is.Equal(comps["backend"].TeamName, "platform")
	is.Equal(comps["backend"].RepoIDs, []string{"PROJ/backend-api"})
}

// TestAddRepoOnboarding_Idempotent verifies re-applying the same event does not
// duplicate the repo in the component's membership.
func TestAddRepoOnboarding_Idempotent(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	o := model.RepoOnboarding{RepoID: "PROJ/backend-api", Component: "backend", Team: "platform"}
	is.NoErr(r.AddRepoOnboarding(ctx, o))
	is.NoErr(r.AddRepoOnboarding(ctx, o))

	comps := componentsByName(t, r)
	is.Equal(comps["backend"].RepoIDs, []string{"PROJ/backend-api"})
}

// TestAddRepoOnboarding_Merge verifies several repos onboarding to the same
// component name are merged into one component.
func TestAddRepoOnboarding_Merge(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	is.NoErr(r.AddRepoOnboarding(ctx, model.RepoOnboarding{RepoID: "PROJ/api-a", Component: "backend", Team: "platform"}))
	is.NoErr(r.AddRepoOnboarding(ctx, model.RepoOnboarding{RepoID: "PROJ/api-b", Component: "backend", Team: "platform"}))

	comps := componentsByName(t, r)
	is.Equal(len(comps), 1)
	got := append([]string{}, comps["backend"].RepoIDs...)
	sort.Strings(got)
	is.Equal(got, []string{"PROJ/api-a", "PROJ/api-b"})
}

// TestAddRepoOnboarding_Reassign verifies re-publishing a repo under a
// different component moves it: it is removed from the old component and the
// new component's team wins.
func TestAddRepoOnboarding_Reassign(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	is.NoErr(r.AddRepoOnboarding(ctx, model.RepoOnboarding{RepoID: "PROJ/svc", Component: "old", Team: "team-a"}))
	is.NoErr(r.AddRepoOnboarding(ctx, model.RepoOnboarding{RepoID: "PROJ/svc", Component: "new", Team: "team-b"}))

	comps := componentsByName(t, r)
	is.Equal(len(comps["old"].RepoIDs), 0)
	is.Equal(comps["new"].RepoIDs, []string{"PROJ/svc"})
	is.Equal(comps["new"].TeamName, "team-b")

	// The reassignment must be reflected in the ownership chain: a workflow on
	// the repo now resolves to the new component/team.
	addWorkflow(t, r, model.Workflow{ID: "wf", Name: "Build", RepoIDs: []string{"PROJ/svc"}})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run", WorkflowID: "wf", Status: "SUCCESS", Duration: 10})
	teams := runAggregateField(t, r, query.EntityWorkflowRun, query.RunTeamName)
	is.True(teams["team-b"])
	is.True(!teams["team-a"])
}
