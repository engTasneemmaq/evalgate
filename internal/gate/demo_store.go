package gate

import (
	"context"
	"sync"
	"time"
)

type demoSession struct {
	runs    []Run
	touched time.Time
}

// DemoStore keeps a bounded amount of temporary data for the public demo.
// Data is scoped to an unguessable browser session ID and is never persisted.
type DemoStore struct {
	mu          sync.Mutex
	sessions    map[string]*demoSession
	maxSessions int
	maxRuns     int
	ttl         time.Duration
	now         func() time.Time
}

func NewDemoStore(maxSessions, maxRuns int, ttl time.Duration) *DemoStore {
	return &DemoStore{
		sessions:    make(map[string]*demoSession),
		maxSessions: max(1, maxSessions),
		maxRuns:     max(1, maxRuns),
		ttl:         ttl,
		now:         time.Now,
	}
}

func (s *DemoStore) prune(now time.Time) {
	if s.ttl > 0 {
		for id, session := range s.sessions {
			if now.Sub(session.touched) > s.ttl {
				delete(s.sessions, id)
			}
		}
	}
	for len(s.sessions) >= s.maxSessions {
		var oldestID string
		var oldest time.Time
		for id, session := range s.sessions {
			if oldestID == "" || session.touched.Before(oldest) {
				oldestID, oldest = id, session.touched
			}
		}
		delete(s.sessions, oldestID)
	}
}

func (s *DemoStore) List(_ context.Context, sessionID string) ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return []Run{}, nil
	}
	session.touched = s.now()
	return append([]Run(nil), session.runs...), nil
}

func (s *DemoStore) Add(_ context.Context, sessionID string, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.prune(now)
		session = &demoSession{}
		s.sessions[sessionID] = session
	}
	session.touched = now
	session.runs = append([]Run{run}, session.runs...)
	if len(session.runs) > s.maxRuns {
		session.runs = session.runs[:s.maxRuns]
	}
	return nil
}

func (s *DemoStore) Review(_ context.Context, sessionID, runID, caseID, decision string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return Run{}, ErrNotFound
	}
	for i := range session.runs {
		if session.runs[i].ID == runID {
			if err := review(&session.runs[i], caseID, decision); err != nil {
				return Run{}, err
			}
			session.touched = s.now()
			return session.runs[i], nil
		}
	}
	return Run{}, ErrNotFound
}
