package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	gonats "github.com/nats-io/nats.go"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// QueryPort subscribes to core NATS query subjects and streams entity results
// back to the caller's reply inbox. It uses core NATS (not JetStream) because
// request-reply is a core NATS feature.
type QueryPort struct {
	logger  *slog.Logger
	handler domain.QueryHandler
}

func NewQueryPort(logger *slog.Logger, handler domain.QueryHandler) *QueryPort {
	return &QueryPort{logger: logger, handler: handler}
}

// Run subscribes to the six query subjects and the aggregate subject, then
// blocks until ctx is cancelled. Subscriptions are drained before returning.
func (p *QueryPort) Run(ctx context.Context, nc *gonats.Conn) error {
	subs := make([]*gonats.Subscription, 0, 7)

	type entry struct {
		subject string
		handler gonats.MsgHandler
	}
	entries := []entry{
		{SubjectQueryRepo, p.handleQueryRepos},
		{SubjectQueryCommit, p.handleQueryCommits},
		{SubjectQueryPullRequest, p.handleQueryPullRequests},
		{SubjectQueryCicdWorkflow, p.handleQueryWorkflows},
		{SubjectQueryCicdWorkflowRun, p.handleQueryWorkflowRuns},
		{SubjectQueryCicdTask, p.handleQueryWorkflowTasks},
		{SubjectQueryAggregate, p.handleAggregate},
	}

	for _, e := range entries {
		sub, err := nc.Subscribe(e.subject, e.handler)
		if err != nil {
			return fmt.Errorf("subscribing to %s: %w", e.subject, err)
		}
		p.logger.Info("query handler ready", "subject", e.subject)
		subs = append(subs, sub)
	}

	// Flush ensures all SUB commands have been processed by the server
	// before we signal readiness to callers.
	if err := nc.Flush(); err != nil {
		return fmt.Errorf("flushing subscriptions: %w", err)
	}

	<-ctx.Done()

	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	return nil
}

func (p *QueryPort) handleQueryRepos(msg *gonats.Msg) {
	p.handleQuery(msg, func(ctx context.Context, q query.Query) error {
		return p.handler.QueryRepos(ctx, q, func(r model.Repo) error {
			return sendData(msg, r)
		})
	})
}

func (p *QueryPort) handleQueryCommits(msg *gonats.Msg) {
	p.handleQuery(msg, func(ctx context.Context, q query.Query) error {
		return p.handler.QueryCommits(ctx, q, func(c model.Commit) error {
			return sendData(msg, c)
		})
	})
}

func (p *QueryPort) handleQueryPullRequests(msg *gonats.Msg) {
	p.handleQuery(msg, func(ctx context.Context, q query.Query) error {
		return p.handler.QueryPullRequests(ctx, q, func(pr model.PullRequest) error {
			return sendData(msg, pr)
		})
	})
}

func (p *QueryPort) handleQueryWorkflows(msg *gonats.Msg) {
	p.handleQuery(msg, func(ctx context.Context, q query.Query) error {
		return p.handler.QueryWorkflows(ctx, q, func(w model.Workflow) error {
			return sendData(msg, w)
		})
	})
}

func (p *QueryPort) handleQueryWorkflowRuns(msg *gonats.Msg) {
	p.handleQuery(msg, func(ctx context.Context, q query.Query) error {
		return p.handler.QueryWorkflowRuns(ctx, q, func(r model.WorkflowRun) error {
			return sendData(msg, r)
		})
	})
}

func (p *QueryPort) handleQueryWorkflowTasks(msg *gonats.Msg) {
	p.handleQuery(msg, func(ctx context.Context, q query.Query) error {
		return p.handler.QueryWorkflowTasks(ctx, q, func(t model.WorkflowTask) error {
			return sendData(msg, t)
		})
	})
}

// handleQuery decodes the request, runs fn, then sends a done or error reply.
func (p *QueryPort) handleQuery(
	msg *gonats.Msg,
	fn func(ctx context.Context, q query.Query) error,
) {
	l := p.logger.With("subject", msg.Subject)

	if msg.Reply == "" {
		l.Warn("query request has no reply subject; dropping")
		return
	}

	var q query.Query
	if err := json.Unmarshal(msg.Data, &q); err != nil {
		l.Error("decoding query", "error", err)
		_ = sendError(msg, err)
		return
	}

	// Use a background context: the subscription handler runs in a NATS
	// goroutine with no per-request lifecycle; we rely on the emit callback
	// to propagate back-pressure via error returns.
	if err := fn(context.Background(), q); err != nil {
		l.Error("executing query", "error", err)
		_ = sendError(msg, err)
		return
	}

	if err := sendDone(msg); err != nil {
		l.Error("sending done", "error", err)
	}
}

func (p *QueryPort) handleAggregate(msg *gonats.Msg) {
	l := p.logger.With("subject", msg.Subject)

	if msg.Reply == "" {
		l.Warn("aggregate request has no reply subject; dropping")
		return
	}

	var q query.Query
	if err := json.Unmarshal(msg.Data, &q); err != nil {
		l.Error("decoding aggregate query", "error", err)
		_ = sendError(msg, err)
		return
	}

	result, err := p.handler.Aggregate(context.Background(), q)
	if err != nil {
		l.Error("executing aggregate", "error", err)
		_ = sendError(msg, err)
		return
	}

	if err := sendAggregation(msg, result); err != nil {
		l.Error("sending aggregation result", "error", err)
		return
	}
	if err := sendDone(msg); err != nil {
		l.Error("sending done after aggregation", "error", err)
	}
}

// sendData publishes a single entity result to the request's reply inbox.
func sendData(req *gonats.Msg, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling result: %w", err)
	}
	reply := &gonats.Msg{
		Subject: req.Reply,
		Header:  gonats.Header{HeaderQueryStatus: []string{StatusData}},
		Data:    b,
	}
	return req.RespondMsg(reply)
}

// sendDone publishes the stream terminator to the request's reply inbox.
func sendDone(req *gonats.Msg) error {
	reply := &gonats.Msg{
		Subject: req.Reply,
		Header:  gonats.Header{HeaderQueryStatus: []string{StatusDone}},
	}
	return req.RespondMsg(reply)
}

// sendAggregation publishes the aggregation result tree to the request's reply inbox.
func sendAggregation(req *gonats.Msg, result map[string]query.AggregationResult) error {
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshalling aggregation result: %w", err)
	}
	reply := &gonats.Msg{
		Subject: req.Reply,
		Header:  gonats.Header{HeaderQueryStatus: []string{StatusAggregation}},
		Data:    b,
	}
	return req.RespondMsg(reply)
}

// sendError publishes an error terminator to the request's reply inbox.
func sendError(req *gonats.Msg, queryErr error) error {
	body, _ := json.Marshal(map[string]string{"error": queryErr.Error()})
	reply := &gonats.Msg{
		Subject: req.Reply,
		Header:  gonats.Header{HeaderQueryStatus: []string{StatusError}},
		Data:    body,
	}
	return req.RespondMsg(reply)
}
