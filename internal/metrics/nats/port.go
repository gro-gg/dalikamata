package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const (
	consumerName = "metrics-git-pullrequest"
)

type Port struct {
	natsURL string
	logger  *slog.Logger
}

func NewPort(logger *slog.Logger, natsURL string) *Port {
	p := &Port{
		natsURL: natsURL,
		logger:  logger,
	}

	return p
}

func (p *Port) Subscribe(ctx context.Context, handler func(model.PullRequest)) error {
	nc, err := nats.Connect(p.natsURL)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return fmt.Errorf("creating jetstream: %w", err)
	}

	stream, err := js.Stream(ctx, dalinats.StreamIngestName)
	if err != nil {
		nc.Close()
		return fmt.Errorf("getting %s stream: %w", dalinats.StreamIngestName, err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: dalinats.SubjectPullRequest,
		MaxDeliver:    dalinats.MaxDeliver,
	})
	if err != nil {
		nc.Close()
		return fmt.Errorf("creating consumer %s: %w", consumerName, err)
	}

	consumeCtx, err := consumer.Consume(p.pullRequestHandler(handler))
	if err != nil {
		nc.Close()
		return fmt.Errorf("starting consumer: %w", err)
	}

	go func() {
		<-ctx.Done()
		consumeCtx.Drain()
		nc.Close()
	}()

	return nil
}

func (p *Port) pullRequestHandler(handler func(model.PullRequest)) func(jetstream.Msg) {
	l := p.logger.With("subject", dalinats.SubjectPullRequest).With("consumer", consumerName)

	return func(msg jetstream.Msg) {
		l.Debug("pullRequestHandler: received message", "message", string(msg.Data()))
		var pr model.PullRequest
		data := msg.Data()
		if err := json.Unmarshal(data, &pr); err != nil {
			l.Error("unmarshalling pull request", "message", string(data), "error", err)
			if err := msg.Nak(); err != nil {
				l.Error("nak message", "error", err)
			}
			return
		}
		handler(pr)
		if err := msg.Ack(); err != nil {
			l.Error("ack message", "error", err)
		}
	}
}
