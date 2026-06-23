package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	gonats "github.com/nats-io/nats.go"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

const defaultQueryTimeout = 30 * time.Second

// QueryClientOpt configures a QueryClient.
type QueryClientOpt func(*QueryClient)

// WithQueryTimeout overrides the default per-query timeout (30s).
func WithQueryTimeout(d time.Duration) QueryClientOpt {
	return func(c *QueryClient) { c.timeout = d }
}

// QueryClient sends typed queries to a running QueryPort and receives
// streamed results over core NATS request-reply.
type QueryClient struct {
	nc      *gonats.Conn
	logger  *slog.Logger
	timeout time.Duration
}

func NewQueryClient(nc *gonats.Conn, logger *slog.Logger, opts ...QueryClientOpt) *QueryClient {
	c := &QueryClient{nc: nc, logger: logger, timeout: defaultQueryTimeout}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Streaming API ----------------------------------------------------------

// QueryRepos streams matching Repo entities. The channel is closed when the
// server sends the done sentinel or when ctx is cancelled.
func (c *QueryClient) QueryRepos(ctx context.Context, q query.Query) (<-chan model.Repo, <-chan error) {
	return streamQuery[model.Repo](ctx, c, SubjectQueryRepo, q)
}

// QueryCommits streams matching Commit entities.
func (c *QueryClient) QueryCommits(ctx context.Context, q query.Query) (<-chan model.Commit, <-chan error) {
	return streamQuery[model.Commit](ctx, c, SubjectQueryCommit, q)
}

// QueryPullRequests streams matching PullRequest entities.
func (c *QueryClient) QueryPullRequests(ctx context.Context, q query.Query) (<-chan model.PullRequest, <-chan error) {
	return streamQuery[model.PullRequest](ctx, c, SubjectQueryPullRequest, q)
}

// QueryWorkflows streams matching Workflow entities.
func (c *QueryClient) QueryWorkflows(ctx context.Context, q query.Query) (<-chan model.Workflow, <-chan error) {
	return streamQuery[model.Workflow](ctx, c, SubjectQueryCicdWorkflow, q)
}

// QueryWorkflowRuns streams matching WorkflowRun entities.
func (c *QueryClient) QueryWorkflowRuns(ctx context.Context, q query.Query) (<-chan model.WorkflowRun, <-chan error) {
	return streamQuery[model.WorkflowRun](ctx, c, SubjectQueryCicdWorkflowRun, q)
}

// QueryWorkflowTasks streams matching WorkflowTask entities.
func (c *QueryClient) QueryWorkflowTasks(ctx context.Context, q query.Query) (<-chan model.WorkflowTask, <-chan error) {
	return streamQuery[model.WorkflowTask](ctx, c, SubjectQueryCicdTask, q)
}

// QueryTeams streams matching Team entities.
func (c *QueryClient) QueryTeams(ctx context.Context, q query.Query) (<-chan model.Team, <-chan error) {
	return streamQuery[model.Team](ctx, c, SubjectQueryPlatformTeam, q)
}

// QueryComponents streams matching Component entities.
func (c *QueryClient) QueryComponents(ctx context.Context, q query.Query) (<-chan model.Component, <-chan error) {
	return streamQuery[model.Component](ctx, c, SubjectQueryPlatformComponent, q)
}

// --- Collecting helpers -----------------------------------------------------

// QueryReposAll collects all matching Repo entities into a slice.
func (c *QueryClient) QueryReposAll(ctx context.Context, q query.Query) ([]model.Repo, error) {
	out, errs := c.QueryRepos(ctx, q)
	return collectAll(out, errs)
}

// QueryCommitsAll collects all matching Commit entities into a slice.
func (c *QueryClient) QueryCommitsAll(ctx context.Context, q query.Query) ([]model.Commit, error) {
	out, errs := c.QueryCommits(ctx, q)
	return collectAll(out, errs)
}

// QueryPullRequestsAll collects all matching PullRequest entities into a slice.
func (c *QueryClient) QueryPullRequestsAll(ctx context.Context, q query.Query) ([]model.PullRequest, error) {
	out, errs := c.QueryPullRequests(ctx, q)
	return collectAll(out, errs)
}

// QueryWorkflowsAll collects all matching Workflow entities into a slice.
func (c *QueryClient) QueryWorkflowsAll(ctx context.Context, q query.Query) ([]model.Workflow, error) {
	out, errs := c.QueryWorkflows(ctx, q)
	return collectAll(out, errs)
}

// QueryWorkflowRunsAll collects all matching WorkflowRun entities into a slice.
func (c *QueryClient) QueryWorkflowRunsAll(ctx context.Context, q query.Query) ([]model.WorkflowRun, error) {
	out, errs := c.QueryWorkflowRuns(ctx, q)
	return collectAll(out, errs)
}

// QueryWorkflowTasksAll collects all matching WorkflowTask entities into a slice.
func (c *QueryClient) QueryWorkflowTasksAll(ctx context.Context, q query.Query) ([]model.WorkflowTask, error) {
	out, errs := c.QueryWorkflowTasks(ctx, q)
	return collectAll(out, errs)
}

// QueryTeamsAll collects all matching Team entities into a slice.
func (c *QueryClient) QueryTeamsAll(ctx context.Context, q query.Query) ([]model.Team, error) {
	out, errs := c.QueryTeams(ctx, q)
	return collectAll(out, errs)
}

// QueryComponentsAll collects all matching Component entities into a slice.
func (c *QueryClient) QueryComponentsAll(ctx context.Context, q query.Query) ([]model.Component, error) {
	out, errs := c.QueryComponents(ctx, q)
	return collectAll(out, errs)
}

// --- Aggregation ------------------------------------------------------------

// Aggregate sends q to the server-side aggregation handler and returns the
// named aggregation result tree. The query must include at least one entry in
// q.Aggs; an empty Aggs returns nil without an error.
func (c *QueryClient) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("marshalling aggregate query: %w", err)
	}

	inbox := c.nc.NewInbox()
	sub, err := c.nc.SubscribeSync(inbox)
	if err != nil {
		return nil, fmt.Errorf("subscribing to inbox: %w", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	if err := c.nc.PublishRequest(SubjectQueryAggregate, inbox, body); err != nil {
		return nil, fmt.Errorf("publishing aggregate query: %w", err)
	}

	deadline := time.Now().Add(c.timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("aggregate query timed out after %s", c.timeout)
		}

		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("receiving aggregate reply: %w", err)
		}

		switch msg.Header.Get(HeaderQueryStatus) {
		case StatusDone:
			return nil, nil
		case StatusError:
			var payload struct {
				Error string `json:"error"`
			}
			if e := json.Unmarshal(msg.Data, &payload); e != nil || payload.Error == "" {
				return nil, fmt.Errorf("server error (unparseable)")
			}
			return nil, fmt.Errorf("server error: %s", payload.Error)
		case StatusAggregation:
			var result map[string]query.AggregationResult
			if e := json.Unmarshal(msg.Data, &result); e != nil {
				return nil, fmt.Errorf("decoding aggregation result: %w", e)
			}
			return result, nil
		default:
			c.logger.Warn("unknown status in aggregate reply", "status", msg.Header.Get(HeaderQueryStatus))
		}
	}
}

// --- Generic internals ------------------------------------------------------

// streamQuery is the shared implementation for all streaming methods.
// It creates a sync subscription on a fresh inbox BEFORE publishing the
// request so that no reply message can be missed. The goroutine is cleaned up
// on either server done/error or ctx cancellation.
func streamQuery[T any](ctx context.Context, c *QueryClient, subject string, q query.Query) (<-chan T, <-chan error) {
	out := make(chan T)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		body, err := json.Marshal(q)
		if err != nil {
			errs <- fmt.Errorf("marshalling query: %w", err)
			return
		}

		inbox := c.nc.NewInbox()
		sub, err := c.nc.SubscribeSync(inbox)
		if err != nil {
			errs <- fmt.Errorf("subscribing to inbox: %w", err)
			return
		}
		defer sub.Unsubscribe() //nolint:errcheck

		if err := c.nc.PublishRequest(subject, inbox, body); err != nil {
			errs <- fmt.Errorf("publishing query: %w", err)
			return
		}

		deadline := time.Now().Add(c.timeout)

		for {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				errs <- fmt.Errorf("query timed out after %s", c.timeout)
				return
			}

			msg, err := sub.NextMsgWithContext(ctx)
			if err != nil {
				if ctx.Err() != nil {
					errs <- ctx.Err()
				} else {
					errs <- fmt.Errorf("receiving reply: %w", err)
				}
				return
			}

			status := msg.Header.Get(HeaderQueryStatus)
			switch status {
			case StatusDone:
				return
			case StatusError:
				var payload struct {
					Error string `json:"error"`
				}
				if e := json.Unmarshal(msg.Data, &payload); e != nil || payload.Error == "" {
					errs <- fmt.Errorf("server error (unparseable)")
				} else {
					errs <- fmt.Errorf("server error: %s", payload.Error)
				}
				return
			case StatusData:
				var item T
				if e := json.Unmarshal(msg.Data, &item); e != nil {
					errs <- fmt.Errorf("decoding result: %w", e)
					return
				}
				select {
				case out <- item:
				case <-ctx.Done():
					errs <- ctx.Err()
					return
				}
			default:
				c.logger.Warn("unknown query status header", "status", status)
			}
		}
	}()

	return out, errs
}

// collectAll drains a streaming query into a slice.
func collectAll[T any](out <-chan T, errs <-chan error) ([]T, error) {
	var items []T
	for item := range out {
		items = append(items, item)
	}
	if err := <-errs; err != nil {
		return nil, err
	}
	return items, nil
}
