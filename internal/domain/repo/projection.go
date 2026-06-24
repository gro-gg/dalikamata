package repo

import (
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	q "codeberg.org/aeforged/dalikamata/internal/domain/query"
)

// projectRepo converts a Repo to a field map for query evaluation.
// Field names match the JSON tags on model.Repo.
func projectRepo(r model.Repo) map[string]any {
	return map[string]any{
		q.RepoID:   r.RepoID,
		q.RepoName: r.Name,
	}
}

// projectCommit converts a Commit to a field map for query evaluation.
func projectCommit(c model.Commit) map[string]any {
	return map[string]any{
		q.CommitSHA:       c.SHA,
		q.CommitRepoID:    c.RepoID,
		q.CommitAuthor:    c.Author,
		q.CommitTimestamp: c.Timestamp,
	}
}

// projectPullRequest converts a PullRequest to a field map for query evaluation.
// now is used to compute cycle_time_seconds for OPEN PRs; pass MemoryRepository.clock().
func projectPullRequest(pr model.PullRequest, now time.Time) map[string]any {
	return map[string]any{
		q.PRID:               pr.ID,
		q.PRRepoID:           pr.RepoID,
		q.PRName:             pr.Name,
		q.PRTitle:            pr.Title,
		q.PRDescription:      pr.Description,
		q.PRState:            pr.State,
		q.PRAuthor:           pr.Author,
		q.PRCreatedAt:        pr.CreatedAt,
		q.PRUpdatedAt:        pr.UpdatedAt,
		q.PRCycleTimeSeconds: prCycleTimeSeconds(pr, now),
	}
}

// prCycleTimeSeconds returns the elapsed seconds from PR creation to its
// final state (MERGED/DECLINED) or to now for OPEN PRs.
func prCycleTimeSeconds(pr model.PullRequest, now time.Time) float64 {
	end := now
	switch pr.State {
	case model.PullRequestStateMerged, model.PullRequestStateDeclined:
		end = pr.UpdatedAt
	}
	return end.Sub(pr.CreatedAt).Seconds()
}

// projectWorkflow converts a Workflow to a field map for query evaluation.
func projectWorkflow(w model.Workflow) map[string]any {
	return map[string]any{
		q.WorkflowID:     w.ID,
		q.WorkflowName:   w.Name,
		q.WorkflowRepoID: w.RepoID,
	}
}

// ownerLookup carries the closures needed to enrich WorkflowRun and
// WorkflowTask projections with team/component/workflow-name fields.
// All closures must be safe to call concurrently and without holding any lock,
// because they operate on snapshot data captured under the repository's RLock.
type ownerLookup struct {
	// ownership returns (componentName, teamName) for the given workflowID,
	// falling back to ("unknown", "unknown") for unclaimed workflows.
	ownership func(workflowID string) (component, team string)
	// workflowName returns the human-readable workflow name for the given ID,
	// falling back to the ID itself when no Workflow record exists.
	workflowName func(workflowID string) string
	// diagnose returns the full resolution chain for the given workflowID,
	// indicating at which arm it succeeded or failed.
	diagnose func(workflowID string) model.OwnershipDiagnostics
}

// projectWorkflowRun converts a WorkflowRun to a field map for query evaluation.
// Enriched fields (team_name, component_name, workflow_name) are looked up via lkp.
func projectWorkflowRun(r model.WorkflowRun, lkp ownerLookup) map[string]any {
	component, team := lkp.ownership(r.WorkflowID)
	return map[string]any{
		q.RunID:            r.ID,
		q.RunWorkflowID:    r.WorkflowID,
		q.RunNumber:        r.Number,
		q.RunStatus:        r.Status,
		q.RunBranch:        r.Branch,
		q.RunCommitSHA:     r.CommitSHA,
		q.RunStartedAt:     r.StartedAt,
		q.RunDuration:      r.Duration,
		q.RunWorkflowName:  lkp.workflowName(r.WorkflowID),
		q.RunComponentName: component,
		q.RunTeamName:      team,
	}
}

// projectWorkflowTask converts a WorkflowTask to a field map for query evaluation.
// Enriched fields are looked up via runs (task→run→workflowID) and then lkp.
func projectWorkflowTask(t model.WorkflowTask, runs map[string]model.WorkflowRun, lkp ownerLookup) map[string]any {
	workflowID := ""
	branch := ""
	if run, ok := runs[t.WorkflowRunID]; ok {
		workflowID = run.WorkflowID
		branch = run.Branch
	}
	component, team := lkp.ownership(workflowID)
	return map[string]any{
		q.TaskWorkflowRunID: t.WorkflowRunID,
		q.TaskOrder:         t.Order,
		q.TaskName:          t.Name,
		q.TaskStatus:        t.Status,
		q.TaskStartedAt:     t.StartedAt,
		q.TaskDuration:      t.Duration,
		q.TaskWorkflowID:    workflowID,
		q.TaskWorkflowName:  lkp.workflowName(workflowID),
		q.TaskComponentName: component,
		q.TaskTeamName:      team,
		q.TaskBranch:        branch,
	}
}

// projectTeam converts a Team to a field map for query evaluation.
func projectTeam(t model.Team) map[string]any {
	return map[string]any{
		q.TeamName: t.Name,
	}
}

// ensureUnknownTeam appends a synthetic {Name:"unknown"} team to snap if one
// is not already present. Team queries always include this entry so that
// dashboards and filters that reference the fallback ownership label work even
// when no workflow has resolved owners yet.
func ensureUnknownTeam(snap []model.Team) []model.Team {
	for _, t := range snap {
		if t.Name == "unknown" {
			return snap
		}
	}
	return append(snap, model.Team{Name: "unknown"})
}

// projectComponent converts a Component to a field map for query evaluation.
// Nested slice fields (repos, workflows) are omitted from the projection in v1;
// top-level scalar filters are sufficient for initial use cases.
func projectComponent(c model.Component) map[string]any {
	return map[string]any{
		q.ComponentName:     c.Name,
		q.ComponentTeamName: c.TeamName,
	}
}
