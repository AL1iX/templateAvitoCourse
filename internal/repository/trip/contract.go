package trip

import (
	"context"

	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres/transaction"
)

type executorProvider interface {
	Executor(ctx context.Context) transaction.Executor
}
