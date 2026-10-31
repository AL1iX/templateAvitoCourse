package trip

import "time"

type Repository struct {
	db     executorProvider
	keyTTL time.Duration
}

// New returns a repository whose idempotency keys stay live for keyTTL.
func New(db executorProvider, keyTTL time.Duration) *Repository {
	return &Repository{db: db, keyTTL: keyTTL}
}
