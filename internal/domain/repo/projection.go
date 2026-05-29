package repo

import (
	"time"

	q "codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
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
		q.WorkflowID:   w.ID,
		q.WorkflowName: w.Name,
	}
}

// projectWorkflowRun converts a WorkflowRun to a field map for query evaluation.
func projectWorkflowRun(r model.WorkflowRun) map[string]any {
	return map[string]any{
		q.RunID:         r.ID,
		q.RunWorkflowID: r.WorkflowID,
		q.RunNumber:     r.Number,
		q.RunStatus:     r.Status,
		q.RunBranch:     r.Branch,
		q.RunCommitSHA:  r.CommitSHA,
		q.RunStartedAt:  r.StartedAt,
		q.RunDuration:   r.Duration,
	}
}

// projectWorkflowTask converts a WorkflowTask to a field map for query evaluation.
func projectWorkflowTask(t model.WorkflowTask) map[string]any {
	return map[string]any{
		q.TaskWorkflowRunID: t.WorkflowRunID,
		q.TaskOrder:         t.Order,
		q.TaskName:          t.Name,
		q.TaskStatus:        t.Status,
		q.TaskStartedAt:     t.StartedAt,
		q.TaskDuration:      t.Duration,
	}
}

// projectTeam converts a Team to a field map for query evaluation.
func projectTeam(t model.Team) map[string]any {
	return map[string]any{
		q.TeamName: t.Name,
	}
}

// projectComponent converts a Component to a field map for query evaluation.
// Nested slice fields (repos, workflows, artifacts) are omitted from the
// projection in v1; top-level scalar filters are sufficient for initial use cases.
func projectComponent(c model.Component) map[string]any {
	return map[string]any{
		q.ComponentName:     c.Name,
		q.ComponentTeamName: c.TeamName,
	}
}
