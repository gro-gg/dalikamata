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

	SubjectPipelineJob   = "ingest.pipeline.job"
	SubjectPipelineBuild = "ingest.pipeline.build"
	SubjectPipelineStage = "ingest.pipeline.stage"

	DefaultHost = "0.0.0.0"
	DefaultPort = 4222

	LogReceivedMessage = "received message"
)

type NATSPort struct {
	logger          *slog.Logger
	gitHandler      domain.GitEventHandler
	pipelineHandler domain.PipelineEventHandler
}

func NATSConnectionString(natsHost string, natsPort int) string {
	return fmt.Sprintf("nats://%s:%d", natsHost, natsPort)
}

type HandlerOpt func(*NATSPort) error

func WithGitEventHandler(handler domain.GitEventHandler) HandlerOpt {
	return func(p *NATSPort) error {
		p.gitHandler = handler
		return nil
	}
}

func WithPipelineEventHandler(handler domain.PipelineEventHandler) HandlerOpt {
	return func(p *NATSPort) error {
		p.pipelineHandler = handler
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

	pipelineJobConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-pipeline-job",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectPipelineJob,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-pipeline-job consumer: %w", err)
	}

	pipelineBuildConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-pipeline-build",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectPipelineBuild,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-pipeline-build consumer: %w", err)
	}

	pipelineStageConsumer, err := ingestStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "ingest-pipeline-stage",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: SubjectPipelineStage,
		MaxDeliver:    MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("creating ingest-pipeline-stage consumer: %w", err)
	}

	gitRepoConsumeCtx, err := gitRepoConsumer.Consume(s.gitRepoHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectRepo, err)
	}

	gitCommitConsumeCtx, err := gitCommitConsumer.Consume(s.gitCommitHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectCommit, err)
	}

	gitPRConsumeCtx, err := gitPRConsumer.Consume(s.gitPullRequestHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectPullRequest, err)
	}

	pipelineJobConsumeCtx, err := pipelineJobConsumer.Consume(s.pipelineJobHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectPipelineJob, err)
	}

	pipelineBuildConsumeCtx, err := pipelineBuildConsumer.Consume(s.pipelineBuildHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectPipelineBuild, err)
	}

	pipelineStageConsumeCtx, err := pipelineStageConsumer.Consume(s.pipelineStageHandler(ctx))
	if err != nil {
		return fmt.Errorf("starting %s consumer: %w", SubjectPipelineStage, err)
	}

	<-ctx.Done()

	gitRepoConsumeCtx.Drain()
	gitCommitConsumeCtx.Drain()
	gitPRConsumeCtx.Drain()
	pipelineJobConsumeCtx.Drain()
	pipelineBuildConsumeCtx.Drain()
	pipelineStageConsumeCtx.Drain()

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

func (s *NATSPort) pipelineJobHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPipelineJob)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var job model.Job
		if err := json.Unmarshal(msg.Data(), &job); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.pipelineHandler.HandleJob(ctx, job); err != nil {
			l.Error("handling job", "job_id", job.JobID, "error", err)
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

func (s *NATSPort) pipelineBuildHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPipelineBuild)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var build model.Build
		if err := json.Unmarshal(msg.Data(), &build); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.pipelineHandler.HandleBuild(ctx, build); err != nil {
			l.Error("handling build", "build_id", build.ID, "error", err)
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

func (s *NATSPort) pipelineStageHandler(ctx context.Context) func(msg jetstream.Msg) {
	l := s.logger.With("subject", SubjectPipelineStage)

	return func(msg jetstream.Msg) {
		l.Debug(LogReceivedMessage)
		var stage model.PipelineStage
		if err := json.Unmarshal(msg.Data(), &stage); err != nil {
			l.Error("unmarshalling message", "message", string(msg.Data()), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		if err := s.pipelineHandler.HandlePipelineStage(ctx, stage); err != nil {
			l.Error("handling pipeline stage", "build_id", stage.BuildID, "name", stage.Name, "error", err)
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
