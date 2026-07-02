package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/nats-io/nats.go/jetstream"

	"codeberg.org/aeforged/dalikamata/internal/domain"
)

const (
	MaxDeliver = 5

	LogReceivedMessage  = "received message"
	LogHandlerSettingUp = "setting up handler"
)

type NATSPort struct {
	logger          *slog.Logger
	gitHandler      domain.GitEventHandler
	cicdHandler     domain.CicdEventHandler
	platformHandler domain.PlatformEventHandler
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

func WithPlatformEventHandler(handler domain.PlatformEventHandler) HandlerOpt {
	return func(p *NATSPort) error {
		p.platformHandler = handler
		return nil
	}
}

func NewPort(logger *slog.Logger, handlers ...HandlerOpt) (*NATSPort, error) {
	port := &NATSPort{
		logger: logger.With("type", "port", "component", "ingest", "connection", "nats"),
	}
	for _, handler := range handlers {
		if err := handler(port); err != nil {
			return nil, err
		}
	}
	if port.gitHandler == nil {
		return nil, fmt.Errorf("git event handler is required")
	}
	if port.cicdHandler == nil {
		return nil, fmt.Errorf("cicd event handler is required")
	}
	return port, nil
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

	var platformTeamConsumeCtx, platformComponentConsumeCtx, platformRepoConsumeCtx jetstream.ConsumeContext
	if s.platformHandler != nil {
		platformTeamConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       "ingest-platform-team",
			AckPolicy:     jetstream.AckExplicitPolicy,
			FilterSubject: SubjectPlatformTeam,
			MaxDeliver:    MaxDeliver,
		})
		if err != nil {
			return fmt.Errorf("creating ingest-platform-team consumer: %w", err)
		}

		platformComponentConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       "ingest-platform-component",
			AckPolicy:     jetstream.AckExplicitPolicy,
			FilterSubject: SubjectPlatformComponent,
			MaxDeliver:    MaxDeliver,
		})
		if err != nil {
			return fmt.Errorf("creating ingest-platform-component consumer: %w", err)
		}

		platformTeamConsumeCtx, err = platformTeamConsumer.Consume(s.platformTeamHandler(ctx))
		if err != nil {
			return fmt.Errorf("starting %s consumer: %w", SubjectPlatformTeam, err)
		}
		s.logger.Debug(LogHandlerSettingUp, "subject", SubjectPlatformTeam)

		platformComponentConsumeCtx, err = platformComponentConsumer.Consume(s.platformComponentHandler(ctx))
		if err != nil {
			return fmt.Errorf("starting %s consumer: %w", SubjectPlatformComponent, err)
		}
		s.logger.Debug(LogHandlerSettingUp, "subject", SubjectPlatformComponent)

		platformRepoConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       "ingest-platform-repo",
			AckPolicy:     jetstream.AckExplicitPolicy,
			FilterSubject: SubjectPlatformRepo,
			MaxDeliver:    MaxDeliver,
		})
		if err != nil {
			return fmt.Errorf("creating ingest-platform-repo consumer: %w", err)
		}

		platformRepoConsumeCtx, err = platformRepoConsumer.Consume(s.platformRepoHandler(ctx))
		if err != nil {
			return fmt.Errorf("starting %s consumer: %w", SubjectPlatformRepo, err)
		}
		s.logger.Debug(LogHandlerSettingUp, "subject", SubjectPlatformRepo)
	}

	<-ctx.Done()

	gitRepoConsumeCtx.Drain()
	gitCommitConsumeCtx.Drain()
	gitPRConsumeCtx.Drain()
	cicdWorkflowConsumeCtx.Drain()
	cicdWorkflowRunConsumeCtx.Drain()
	cicdWorkflowTaskConsumeCtx.Drain()
	if platformTeamConsumeCtx != nil {
		platformTeamConsumeCtx.Drain()
	}
	if platformComponentConsumeCtx != nil {
		platformComponentConsumeCtx.Drain()
	}
	if platformRepoConsumeCtx != nil {
		platformRepoConsumeCtx.Drain()
	}

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

func (s *NATSPort) platformTeamHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPlatformTeam)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var team model.Team
		if err := json.Unmarshal(msg.Data(), &team); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.platformHandler.HandleTeam(ctx, team); err != nil {
			l.Error("handling team", "name", team.Name, "error", err)
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

func (s *NATSPort) platformComponentHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPlatformComponent)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var comp model.Component
		if err := json.Unmarshal(msg.Data(), &comp); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.platformHandler.HandleComponent(ctx, comp); err != nil {
			l.Error("handling component", "name", comp.Name, "error", err)
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

func (s *NATSPort) platformRepoHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPlatformRepo)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var o model.RepoOnboarding
		if err := json.Unmarshal(msg.Data(), &o); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.platformHandler.HandleRepoOnboarding(ctx, o); err != nil {
			l.Error("handling repo onboarding", "repo_id", o.RepoID, "component", o.Component, "error", err)
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
