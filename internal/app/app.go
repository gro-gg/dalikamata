package app

import (
	"context"
	"sync"
)

type Service interface {
	Run(context.Context) error
}

type waitGroupKey struct{}

func WithWaitGroup(ctx context.Context, wg *sync.WaitGroup) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if wg == nil {
		wg = new(sync.WaitGroup)
	}

	return context.WithValue(ctx, waitGroupKey{}, wg)
}

func AddWaitGroup(ctx context.Context) context.Context {
	return WithWaitGroup(ctx, nil)
}

func WaitGroupFrom(ctx context.Context) *sync.WaitGroup {
	if wg, ok := ctx.Value(waitGroupKey{}).(*sync.WaitGroup); ok {
		return wg
	}

	return new(sync.WaitGroup)
}

type App interface {
	Run(context.Context, *sync.WaitGroup) error
}
