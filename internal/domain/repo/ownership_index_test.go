package repo_test

import (
	"context"
	"testing"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

// addComponent is a helper that panics on error — keeps table-driven tests terse.
func addComponent(t *testing.T, r *repo.MemoryRepository, c model.Component) {
	t.Helper()
	if err := r.AddComponent(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func addWorkflow(t *testing.T, r *repo.MemoryRepository, w model.Workflow) {
	t.Helper()
	if err := r.AddWorkflow(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

func addWorkflowRun(t *testing.T, r *repo.MemoryRepository, run model.WorkflowRun) {
	t.Helper()
	if err := r.AddWorkflowRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}

func addWorkflowTask(t *testing.T, r *repo.MemoryRepository, task model.WorkflowTask) {
	t.Helper()
	if err := r.AddWorkflowTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}

// runAggregateField aggregates by terms on field over workflowRun or
// workflowTask entities and returns the set of distinct keys observed.
func runAggregateField(t *testing.T, r *repo.MemoryRepository, entity query.Entity, field string) map[string]bool {
	t.Helper()
	result, err := r.Aggregate(context.Background(), query.Query{
		Entity:   entity,
		AggsOnly: true,
		Aggs: map[string]query.Aggregation{
			"by_field": {Op: query.AggTerms, Field: field},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]bool)
	for _, b := range result["by_field"].Buckets {
		keys[b.Key.(string)] = true
	}
	return keys
}

// TestOwnershipIndex_ComponentBeforeWorkflows verifies that when a component is
// ingested before the workflow/run data, the team_name and component_name fields
// are still populated correctly on the run projection.
func TestOwnershipIndex_ComponentBeforeWorkflows(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	addComponent(t, r, model.Component{
		Name:     "svc-a",
		TeamName: "team-alpha",
		RepoIDs:  []string{"r1"},
	})
	addWorkflow(t, r, model.Workflow{ID: "wf1", Name: "Build Pipeline", RepoID: "r1"})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run1", WorkflowID: "wf1", Status: "SUCCESS", Duration: 120})

	teams := runAggregateField(t, r, query.EntityWorkflowRun, query.RunTeamName)
	is.True(teams["team-alpha"])
	is.True(!teams["unknown"])

	comps := runAggregateField(t, r, query.EntityWorkflowRun, query.RunComponentName)
	is.True(comps["svc-a"])

	wfNames := runAggregateField(t, r, query.EntityWorkflowRun, query.RunWorkflowName)
	is.True(wfNames["Build Pipeline"])

	_ = ctx
}

// TestOwnershipIndex_WorkflowsBeforeComponent verifies that the ownership
// lookup is dynamic: a run ingested before its component is registered still
// surfaces the correct team_name once the component is added.
func TestOwnershipIndex_WorkflowsBeforeComponent(t *testing.T) {
	is := is.New(t)
	r := newRepo()

	addWorkflow(t, r, model.Workflow{ID: "wf2", Name: "Deploy", RepoID: "r2"})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run2", WorkflowID: "wf2", Status: "SUCCESS", Duration: 60})

	// Before the component is registered the team should be unknown.
	teams := runAggregateField(t, r, query.EntityWorkflowRun, query.RunTeamName)
	is.True(teams["unknown"])

	// Now register the owning component.
	addComponent(t, r, model.Component{
		Name:     "svc-b",
		TeamName: "team-beta",
		RepoIDs:  []string{"r2"},
	})

	teams = runAggregateField(t, r, query.EntityWorkflowRun, query.RunTeamName)
	is.True(teams["team-beta"])
	is.True(!teams["unknown"])
}

// TestOwnershipIndex_ComponentOverwriteShrinksList verifies that when a
// component is re-ingested with a smaller workflow list, the removed workflow
// IDs no longer appear in the index.
func TestOwnershipIndex_ComponentOverwriteShrinksList(t *testing.T) {
	is := is.New(t)
	r := newRepo()

	addWorkflow(t, r, model.Workflow{ID: "wf3", Name: "Old Pipeline", RepoID: "r3a"})
	addWorkflow(t, r, model.Workflow{ID: "wf4", Name: "New Pipeline", RepoID: "r3b"})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run3", WorkflowID: "wf3", Status: "SUCCESS", Duration: 30})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run4", WorkflowID: "wf4", Status: "SUCCESS", Duration: 45})

	// Initial registration: both repos owned by "svc-c".
	addComponent(t, r, model.Component{
		Name:     "svc-c",
		TeamName: "team-gamma",
		RepoIDs:  []string{"r3a", "r3b"},
	})

	// Re-ingest with only r3b remaining: wf3 (via r3a) becomes orphaned.
	addComponent(t, r, model.Component{
		Name:     "svc-c",
		TeamName: "team-gamma",
		RepoIDs:  []string{"r3b"},
	})

	teams := runAggregateField(t, r, query.EntityWorkflowRun, query.RunTeamName)
	// run4 (wf4) → team-gamma; run3 (wf3) → unknown after the overwrite.
	is.True(teams["team-gamma"])
	is.True(teams["unknown"])
}

// TestOwnershipIndex_TaskEnrichment verifies that workflow_task projections
// also surface team_name and component_name via the parent run→workflow chain.
func TestOwnershipIndex_TaskEnrichment(t *testing.T) {
	is := is.New(t)
	r := newRepo()

	addComponent(t, r, model.Component{
		Name:     "svc-d",
		TeamName: "team-delta",
		RepoIDs:  []string{"r4"},
	})
	addWorkflow(t, r, model.Workflow{ID: "wf5", Name: "Test Suite", RepoID: "r4"})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run5", WorkflowID: "wf5", Status: "SUCCESS", Duration: 200})
	addWorkflowTask(t, r, model.WorkflowTask{WorkflowRunID: "run5", Name: "unit-tests", Status: "SUCCESS", Duration: 90})
	addWorkflowTask(t, r, model.WorkflowTask{WorkflowRunID: "run5", Name: "lint", Status: "SUCCESS", Duration: 30})

	teams := runAggregateField(t, r, query.EntityWorkflowTask, query.TaskTeamName)
	is.True(teams["team-delta"])
	is.True(!teams["unknown"])

	comps := runAggregateField(t, r, query.EntityWorkflowTask, query.TaskComponentName)
	is.True(comps["svc-d"])

	wfNames := runAggregateField(t, r, query.EntityWorkflowTask, query.TaskWorkflowName)
	is.True(wfNames["Test Suite"])

	wfIDs := runAggregateField(t, r, query.EntityWorkflowTask, query.TaskWorkflowID)
	is.True(wfIDs["wf5"])
}

// TestOwnershipIndex_UnknownFallback verifies that runs/tasks for a workflow
// that has no owning component get team_name="unknown" and component_name="unknown".
func TestOwnershipIndex_UnknownFallback(t *testing.T) {
	is := is.New(t)
	r := newRepo()

	addWorkflow(t, r, model.Workflow{ID: "orphan-wf", Name: "Orphan"})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "orphan-run", WorkflowID: "orphan-wf", Status: "FAILURE", Duration: 5})
	addWorkflowTask(t, r, model.WorkflowTask{WorkflowRunID: "orphan-run", Name: "step", Status: "FAILURE", Duration: 5})

	runTeams := runAggregateField(t, r, query.EntityWorkflowRun, query.RunTeamName)
	is.True(runTeams["unknown"])

	taskTeams := runAggregateField(t, r, query.EntityWorkflowTask, query.TaskTeamName)
	is.True(taskTeams["unknown"])
}

// TestOwnershipDiagnostics_AllReasons exercises every Reason value of the
// OwnershipDiagnostics returned by MemoryRepository.OwnershipDiagnostics.
func TestOwnershipDiagnostics_AllReasons(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	// "ok" — full chain resolves
	addComponent(t, r, model.Component{Name: "svc-ok", TeamName: "team-ok", RepoIDs: []string{"repo-ok"}})
	addWorkflow(t, r, model.Workflow{ID: "wf-ok", RepoID: "repo-ok"})

	// "missing_repo_id" — workflow has no RepoID
	addWorkflow(t, r, model.Workflow{ID: "wf-no-repo", RepoID: ""})

	// "no_component_for_repo" — RepoID set but no component claims it
	addWorkflow(t, r, model.Workflow{ID: "wf-no-comp", RepoID: "repo-unowned"})

	// "no_team_for_component" — component exists but TeamName is empty
	addComponent(t, r, model.Component{Name: "svc-noteam", TeamName: "", RepoIDs: []string{"repo-noteam"}})
	addWorkflow(t, r, model.Workflow{ID: "wf-noteam", RepoID: "repo-noteam"})

	diags, err := r.OwnershipDiagnostics(ctx)
	is.NoErr(err)

	byWF := make(map[string]model.OwnershipDiagnostics, len(diags))
	for _, d := range diags {
		byWF[d.WorkflowID] = d
	}

	is.Equal(byWF["wf-ok"].Reason, "ok")
	is.Equal(byWF["wf-ok"].TeamName, "team-ok")
	is.Equal(byWF["wf-ok"].ComponentName, "svc-ok")
	is.Equal(byWF["wf-ok"].RepoID, "repo-ok")

	is.Equal(byWF["wf-no-repo"].Reason, "missing_repo_id")
	is.Equal(byWF["wf-no-repo"].RepoID, "")

	is.Equal(byWF["wf-no-comp"].Reason, "no_component_for_repo")
	is.Equal(byWF["wf-no-comp"].RepoID, "repo-unowned")

	is.Equal(byWF["wf-noteam"].Reason, "no_team_for_component")
	is.Equal(byWF["wf-noteam"].ComponentName, "svc-noteam")
}

// TestQueryWorkflowRuns_ResolvedTeamName verifies the full projection chain:
// Team → Component(RepoIDs) → Workflow(RepoID) → WorkflowRun carries team_name.
func TestQueryWorkflowRuns_ResolvedTeamName(t *testing.T) {
	is := is.New(t)
	r := newRepo()
	ctx := context.Background()

	if err := r.AddTeam(ctx, model.Team{Name: "team-alpha"}); err != nil {
		t.Fatal(err)
	}
	addComponent(t, r, model.Component{Name: "svc-alpha", TeamName: "team-alpha", RepoIDs: []string{"PROJ/alpha-api"}})
	addWorkflow(t, r, model.Workflow{ID: "alpha-build", Name: "Alpha Build", RepoID: "PROJ/alpha-api"})
	addWorkflowRun(t, r, model.WorkflowRun{ID: "run-alpha-1", WorkflowID: "alpha-build", Status: "SUCCESS", Duration: 90})

	var runs []model.WorkflowRun
	err := r.QueryWorkflowRuns(ctx, query.Query{Entity: query.EntityWorkflowRun}, func(run model.WorkflowRun) error {
		runs = append(runs, run)
		return nil
	})
	is.NoErr(err)
	is.Equal(len(runs), 1)
	is.Equal(runs[0].TeamName, "team-alpha")
	is.Equal(runs[0].ComponentName, "svc-alpha")
	is.Equal(runs[0].WorkflowName, "Alpha Build")
}
