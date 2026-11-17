package cache

// Deliberately bad: proves the deps-nil gate catches the bare factory form.

type Store interface{ Get(key string) string }

type Dependencies struct {
	Store Store
}

type Service struct{ deps *Dependencies }

func New(deps *Dependencies) *Service {
	if deps.Store == nil {
		return nil
	}
	return &Service{deps: deps}
}
