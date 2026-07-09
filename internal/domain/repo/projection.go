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
// RepoIDs is exposed as a []string; evaluator.go matches it by membership
// (any element equal to the filter value).
func projectWorkflow(w model.Workflow) map[string]any {
	return map[string]any{
		q.WorkflowID:      w.ID,
		q.WorkflowName:    w.Name,
		q.WorkflowRepoIDs: w.RepoIDs,
	}
}

// owner is one resolved (component, team) pair of a workflow — a workflow may
// resolve to several when its repos belong to different components. Team is
// "unknown" when the component has no team, but Component is always set (a
// repo that maps to no component contributes no owner at all — see
// newOwnerLookup).
type owner struct {
	Component string
	Team      string
}

// ownerLookup carries the closures needed to enrich WorkflowRun and
// WorkflowTask projections with team/component/workflow-name fields.
// All closures must be safe to call concurrently and without holding any lock,
// because they operate on snapshot data captured under the repository's RLock.
type ownerLookup struct {
	// owners returns the deduplicated (component, team) pairs for the given
	// workflowID, in the workflow's repo publish order, falling back to
	// [{"unknown","unknown"}] when none of its repos resolve to an owner.
	owners func(workflowID string) []owner
	// workflowName returns the human-readable workflow name for the given ID,
	// falling back to the ID itself when no Workflow record exists.
	workflowName func(workflowID string) string
	// diagnose returns the full per-repo resolution chain for the given
	// workflowID, indicating at which arm each repo succeeded or failed.
	diagnose func(workflowID string) model.OwnershipDiagnostics
}

// ownerComponents returns the deduplicated, order-preserving component names
// of owners.
func ownerComponents(owners []owner) []string {
	out := make([]string, 0, len(owners))
	seen := make(map[string]bool, len(owners))
	for _, o := range owners {
		if seen[o.Component] {
			continue
		}
		seen[o.Component] = true
		out = append(out, o.Component)
	}
	return out
}

// ownerTeams returns the deduplicated, order-preserving team names of owners.
func ownerTeams(owners []owner) []string {
	out := make([]string, 0, len(owners))
	seen := make(map[string]bool, len(owners))
	for _, o := range owners {
		if seen[o.Team] {
			continue
		}
		seen[o.Team] = true
		out = append(out, o.Team)
	}
	return out
}

// ownerKeys returns the correlated "team|component" pivot key (see
// q.OwnerKey) for each owner pair, in order. Used for the projection-only
// RunOwner/TaskOwner aggregation field so a terms-agg fan-out cannot mix up
// team/component pairings — see fields.go.
func ownerKeys(owners []owner) []string {
	out := make([]string, len(owners))
	for i, o := range owners {
		out[i] = q.OwnerKey(o.Team, o.Component)
	}
	return out
}

// projectWorkflowRun converts a WorkflowRun to a field map for query evaluation.
// Enriched fields (team_name, component_name, workflow_name, owner) are looked
// up via lkp; team_name/component_name are []string since a workflow can
// resolve to several owners.
func projectWorkflowRun(r model.WorkflowRun, lkp ownerLookup) map[string]any {
	owners := lkp.owners(r.WorkflowID)
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
		q.RunComponentName: ownerComponents(owners),
		q.RunTeamName:      ownerTeams(owners),
		q.RunOwner:         ownerKeys(owners),
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
	owners := lkp.owners(workflowID)
	return map[string]any{
		q.TaskWorkflowRunID: t.WorkflowRunID,
		q.TaskOrder:         t.Order,
		q.TaskName:          t.Name,
		q.TaskStatus:        t.Status,
		q.TaskStartedAt:     t.StartedAt,
		q.TaskDuration:      t.Duration,
		q.TaskWorkflowID:    workflowID,
		q.TaskWorkflowName:  lkp.workflowName(workflowID),
		q.TaskComponentName: ownerComponents(owners),
		q.TaskTeamName:      ownerTeams(owners),
		q.TaskOwner:         ownerKeys(owners),
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
