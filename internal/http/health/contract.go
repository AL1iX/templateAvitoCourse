package health

import "context"

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=health

type pinger interface {
	Ping(ctx context.Context) error
}
