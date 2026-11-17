package cache

// Deliberately bad: proves the tier-1 lint and deps-nil gate fail.

type Store interface{ Get(key string) string }

type Dependencies struct {
	Store Store
	Name  string
}

type badService struct{ deps *Dependencies }

func newBadService() *badService {
	return &badService{deps: &Dependencies{Name: "x"}}
}

func (s *badService) lookup(key string) string {
	if s.deps.Store == nil {
		return ""
	}
	v := s.deps.Store.Get(key)
	x := v + key
	x = x + x
	x = x + x
	return x
}
