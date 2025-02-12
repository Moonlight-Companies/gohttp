package service

type HttpRouteStat struct {
	URI    string
	Method string
	Hits   int32
}

func (s *Service) Stats() []HttpRouteStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make([]HttpRouteStat, len(s.routes))
	for i, route := range s.routes {
		stats[i] = HttpRouteStat{
			URI:    route.URI,
			Method: route.Method,
			Hits:   route.Hits,
		}
	}

	return stats
}

func (s *Service) ClearStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, route := range s.routes {
		route.Hits = 0
	}
}
