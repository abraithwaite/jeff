package memory

import (
	"context"
	"sync"
	"time"

	"github.com/abraithwaite/jeff/v2"
)

type item struct {
	value []jeff.Session
	exp   time.Time
}

var now = func() time.Time {
	return time.Now()
}

// Memory satisfies the jeff.Storage interface
type Memory struct {
	sessions map[string]item
	rw       sync.RWMutex
}

// New initializes a new in-memory Storage for jeff
func New() *Memory {
	return &Memory{
		sessions: make(map[string]item),
	}
}

// Store satisfies the jeff.Store.Store method
func (m *Memory) Store(_ context.Context, key []byte, value []jeff.Session, exp time.Time) error {
	m.rw.Lock()
	m.sessions[string(key)] = item{
		value: value,
		exp:   exp,
	}
	m.rw.Unlock()
	return nil
}

// Fetch satisfies the jeff.Store.Fetch method
func (m *Memory) Fetch(_ context.Context, key []byte) ([]jeff.Session, error) {
	m.rw.RLock()
	v, ok := m.sessions[string(key)]
	m.rw.RUnlock()
	if !ok || v.exp.Before(time.Now()) {
		return nil, nil
	}
	return v.value, nil
}

// Delete satisfies the jeff.Store.Delete method
func (m *Memory) Delete(_ context.Context, key []byte) error {
	m.rw.Lock()
	delete(m.sessions, string(key))
	m.rw.Unlock()
	return nil
}
