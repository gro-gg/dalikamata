package domain

import (
	"context"
)

type Service interface {
	Run(context.Context) error
}
