package repo

import (
	"errors"
	"sync"
)

var ErrNotFound = errors.New("job not found")

// Store is an in-memory job registry safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]Job
}

func NewStore() *Store {
	return &Store{jobs: make(map[string]Job)}
}

func (s *Store) Save(j Job) {
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.mu.Unlock()
}

func (s *Store) Get(id string) (Job, error) {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()
	if !ok {
		return Job{}, ErrNotFound
	}
	return j, nil
}

func (s *Store) Update(id string, fn func(*Job) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if err := fn(&j); err != nil {
		return err
	}
	s.jobs[id] = j
	return nil
}

func (s *Store) List() []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out
}
