package domain_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

var errRepo = errors.New("repository error")

// stubRepository is a Repository + QueryRepository that returns errRepo on
// every write call and panics on any read call (reads are not under test here).
type stubRepository struct {
	err error
}

func (s *stubRepository) AddRepo(_ context.Context, _ model.Repo) error     { return s.err }
func (s *stubRepository) AddCommit(_ context.Context, _ model.Commit) error { return s.err }
func (s *stubRepository) AddPullRequest(_ context.Context, _ model.PullRequest) error {
	return s.err
}
func (s *stubRepository) AddWorkflow(_ context.Context, _ model.Workflow) error { return s.err }
func (s *stubRepository) AddWorkflowRun(_ context.Context, _ model.WorkflowRun) error {
	return s.err
}
func (s *stubRepository) AddWorkflowTask(_ context.Context, _ model.WorkflowTask) error {
	return s.err
}
func (s *stubRepository) AddTeam(_ context.Context, _ model.Team) error           { return s.err }
func (s *stubRepository) AddComponent(_ context.Context, _ model.Component) error { return s.err }

func (s *stubRepository) QueryRepos(_ context.Context, _ query.Query, _ func(model.Repo) error) error {
	return s.err
}
func (s *stubRepository) QueryCommits(_ context.Context, _ query.Query, _ func(model.Commit) error) error {
	return s.err
}
func (s *stubRepository) QueryPullRequests(_ context.Context, _ query.Query, _ func(model.PullRequest) error) error {
	return s.err
}
func (s *stubRepository) QueryWorkflows(_ context.Context, _ query.Query, _ func(model.Workflow) error) error {
	return s.err
}
func (s *stubRepository) QueryWorkflowRuns(_ context.Context, _ query.Query, _ func(model.WorkflowRun) error) error {
	return s.err
}
func (s *stubRepository) QueryWorkflowTasks(_ context.Context, _ query.Query, _ func(model.WorkflowTask) error) error {
	return s.err
}
func (s *stubRepository) QueryTeams(_ context.Context, _ query.Query, _ func(model.Team) error) error {
	return s.err
}
func (s *stubRepository) QueryComponents(_ context.Context, _ query.Query, _ func(model.Component) error) error {
	return s.err
}
func (s *stubRepository) Aggregate(_ context.Context, _ query.Query) (map[string]query.AggregationResult, error) {
	return nil, s.err
}

func newService(err error) *domain.DomainService {
	repo := &stubRepository{err: err}
	return domain.NewDomainService(repo, repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// ---- Handle* error propagation -----------------------------------------------

func TestHandleRepo_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleRepo(context.Background(), model.Repo{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleRepo err = %v, want %v", err, errRepo)
	}
}

func TestHandleCommit_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleCommit(context.Background(), model.Commit{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleCommit err = %v, want %v", err, errRepo)
	}
}

func TestHandlePullRequest_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandlePullRequest(context.Background(), model.PullRequest{}); !errors.Is(err, errRepo) {
		t.Errorf("HandlePullRequest err = %v, want %v", err, errRepo)
	}
}

func TestHandleWorkflow_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleWorkflow(context.Background(), model.Workflow{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleWorkflow err = %v, want %v", err, errRepo)
	}
}

func TestHandleWorkflowRun_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleWorkflowRun(context.Background(), model.WorkflowRun{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleWorkflowRun err = %v, want %v", err, errRepo)
	}
}

func TestHandleWorkflowTask_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleWorkflowTask(context.Background(), model.WorkflowTask{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleWorkflowTask err = %v, want %v", err, errRepo)
	}
}

func TestHandleTeam_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleTeam(context.Background(), model.Team{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleTeam err = %v, want %v", err, errRepo)
	}
}

func TestHandleComponent_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.HandleComponent(context.Background(), model.Component{}); !errors.Is(err, errRepo) {
		t.Errorf("HandleComponent err = %v, want %v", err, errRepo)
	}
}

// ---- Query* error propagation ------------------------------------------------

func TestQueryRepos_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryRepos(context.Background(), query.Query{}, func(model.Repo) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryRepos err = %v, want %v", err, errRepo)
	}
}

func TestQueryCommits_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryCommits(context.Background(), query.Query{}, func(model.Commit) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryCommits err = %v, want %v", err, errRepo)
	}
}

func TestQueryPullRequests_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryPullRequests(context.Background(), query.Query{}, func(model.PullRequest) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryPullRequests err = %v, want %v", err, errRepo)
	}
}

func TestQueryWorkflows_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryWorkflows(context.Background(), query.Query{}, func(model.Workflow) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryWorkflows err = %v, want %v", err, errRepo)
	}
}

func TestQueryWorkflowRuns_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryWorkflowRuns(context.Background(), query.Query{}, func(model.WorkflowRun) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryWorkflowRuns err = %v, want %v", err, errRepo)
	}
}

func TestQueryWorkflowTasks_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryWorkflowTasks(context.Background(), query.Query{}, func(model.WorkflowTask) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryWorkflowTasks err = %v, want %v", err, errRepo)
	}
}

func TestQueryTeams_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryTeams(context.Background(), query.Query{}, func(model.Team) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryTeams err = %v, want %v", err, errRepo)
	}
}

func TestQueryComponents_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	if err := svc.QueryComponents(context.Background(), query.Query{}, func(model.Component) error { return nil }); !errors.Is(err, errRepo) {
		t.Errorf("QueryComponents err = %v, want %v", err, errRepo)
	}
}

func TestAggregate_PropagatesRepoError(t *testing.T) {
	svc := newService(errRepo)
	_, err := svc.Aggregate(context.Background(), query.Query{})
	if !errors.Is(err, errRepo) {
		t.Errorf("Aggregate err = %v, want %v", err, errRepo)
	}
}

// ---- nil-error path (success) ------------------------------------------------

func TestHandleRepo_NilOnSuccess(t *testing.T) {
	svc := newService(nil)
	if err := svc.HandleRepo(context.Background(), model.Repo{RepoID: "r1"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
