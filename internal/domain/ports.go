package domain

import (
	"context"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// Publisher is the outgoing port for emitting Git events.
type Publisher interface {
	PublishCommit(context.Context, model.Commit) error
	PublishPullRequest(context.Context, model.PullRequest) error
	PublishRepo(context.Context, model.Repo) error
}

// PullRequestSubscriber is the incoming port for receiving pull request events.
type PullRequestSubscriber interface {
	Subscribe(ctx context.Context, handler func(model.PullRequest)) error
}
