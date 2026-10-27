package trip

type Repository struct {
	db executorProvider
}

func New(db executorProvider) *Repository {
	return &Repository{db: db}
}
