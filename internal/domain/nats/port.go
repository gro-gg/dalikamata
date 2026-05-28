package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const (
	MaxDeliver = 5

	StreamIngest       = "ingest.>"
	StreamIngestName   = "INGEST"
	SubjectCommit      = "ingest.git.commit"
	SubjectPullRequest = "ingest.git.pullrequest"
	SubjectRepo        = "ingest.git.repo"

	SubjectCicdWorkflow     = "ingest.cicd.workflow"
	SubjectCicdWorkflowRun  = "ingest.cicd.workflowRun"
	SubjectCicdWorkflowTask = "ingest.cicd.workflowTask"

	LogReceivedMessage  = "received message"
	LogHandlerSettingUp = "setting up handler"
)

type NATSPort struct {
	logger      *slog.Logger
	gitHandler  domain.GitEventHandler
	cicdHandler domain.CicdEventHandler
}

type HandlerOpt func(*NATSPort) error

func WithGitEventHandler(handler domain.GitEventHandler) HandlerOpt {
	return func(p *NATSPort) error {
		p.gitHandler = handler
		return nil
	}
}

func WithCicdEventHandler(handler domain.CicdEventHandler) HandlerOpt {
	return func(p *NATSPort) error {
		p.cicdHandler = handler
		return nil
	}
}

func NewPort(logger *slog.Logger, handlers ...HandlerOpt) *NATSPort {
	port := &NATSPort{
		logger: logger.With("type", "port", "component", "ingest", "connection", "nats"),
	}
	for _, handler := range handlers {
		err := handler(port)
		if err != nil {
			port.logger.Error(err.Error())
		}
	}
	return port
}

func (s *NATSPort) Run(ctx context.Context, js jetstream.JetStream) error {
	s.logger.Info("Starting Event Handling")

	ingestStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamIngestName,
		Subjects: []string{StreamIngest},
	})
	if err != nil {
		return fmt.Errorf("creating stream %s: %w", StreamIngestName, err)
	}

	gitRepoConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-repo",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectRepo,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-git-repo consumer: %w", err)
	}

	gitCommitConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-commit",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectCommit,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-git-commit consumer: %w", err)
	}

	gitPRConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-git-pullrequest",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectPullRequest,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-git-pullrequest consumer: %w", err)
	}

	cicdWorkflowConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-cicd-workflow",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectCicdWorkflow,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-cicd-workflow consumer: %w", err)
	}

	cicdWorkflowRunConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-cicd-workflow-run",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectCicdWorkflowRun,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-cicd-workflow-run consumer: %w", err)
	}

	cicdWorkflowTaskConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-cicd-workflow-task",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectCicdWorkflowTask,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-cicd-workflow-task consumer: %w", err)
	}

	gitRepoConsumeCtx, err := gitRepoConsumer.Consume(s.gitRepoHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectRepo, err)
	}
	s.logger.Debug(LogHandlerSettingUp, "subject", SubjectRepo)

	gitCommitConsumeCtx, err := gitCommitConsumer.Consume(s.gitCommitHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectCommit, err)
	}
	s.logger.Debug(LogHandlerSettingUp, "subject", SubjectCommit)

	gitPRConsumeCtx, err := gitPRConsumer.Consume(s.gitPullRequestHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectPullRequest, err)
	}
	s.logger.Debug(LogHandlerSettingUp, "subject", SubjectPullRequest)

	cicdWorkflowConsumeCtx, err := cicdWorkflowConsumer.Consume(s.cicdWorkflowHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectCicdWorkflow, err)
	}
	s.logger.Debug(LogHandlerSettingUp, "subject", SubjectCicdWorkflow)

	cicdWorkflowRunConsumeCtx, err := cicdWorkflowRunConsumer.Consume(s.cicdWorkflowRunHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectCicdWorkflowRun, err)
	}
	s.logger.Debug(LogHandlerSettingUp, "subject", SubjectCicdWorkflowRun)

	cicdWorkflowTaskConsumeCtx, err := cicdWorkflowTaskConsumer.Consume(s.cicdWorkflowTaskHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectCicdWorkflowTask, err)
	}
	s.logger.Debug(LogHandlerSettingUp, "subject", SubjectCicdWorkflowTask)

	<-ctx.Done()

	gitRepoConsumeCtx.Drain()
	gitCommitConsumeCtx.Drain()
	gitPRConsumeCtx.Drain()
	cicdWorkflowConsumeCtx.Drain()
	cicdWorkflowRunConsumeCtx.Drain()
	cicdWorkflowTaskConsumeCtx.Drain()

	s.logger.Info("Event Handling Shut Down")
	return nil
}

func (s *NATSPort) gitRepoHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectRepo)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var repo model.Repo
		if err := json.Unmarshal(msg.Data(), &repo); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.gitHandler.HandleRepo(ctx, repo); err != nil {
			l.Error("handling repo", "repo_id", repo.RepoID, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) gitCommitHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectCommit)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var commit model.Commit
		if err := json.Unmarshal(msg.Data(), &commit); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.gitHandler.HandleCommit(ctx, commit); err != nil {
			l.Error("handling commit", "sha", commit.SHA, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) cicdWorkflowHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectCicdWorkflow)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var workflow model.Workflow
		if err := json.Unmarshal(msg.Data(), &workflow); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.cicdHandler.HandleWorkflow(ctx, workflow); err != nil {
			l.Error("handling workflow", "id", workflow.ID, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) cicdWorkflowRunHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectCicdWorkflowRun)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var run model.WorkflowRun
		if err := json.Unmarshal(msg.Data(), &run); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.cicdHandler.HandleWorkflowRun(ctx, run); err != nil {
			l.Error("handling workflow run", "id", run.ID, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) cicdWorkflowTaskHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectCicdWorkflowTask)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var task model.WorkflowTask
		if err := json.Unmarshal(msg.Data(), &task); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.cicdHandler.HandleWorkflowTask(ctx, task); err != nil {
			l.Error("handling workflow task", "id", task.WorkflowRunID, "name", task.Name, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}

func (s *NATSPort) gitPullRequestHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPullRequest)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var pr model.PullRequest
		if err := json.Unmarshal(msg.Data(), &pr); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.gitHandler.HandlePullRequest(ctx, pr); err != nil {
			l.Error("handling pull request", "id", pr.ID, "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}
