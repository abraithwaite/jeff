package memory

import (
	"context"
	"errors"
	"sync"
	"time"
)

type item struct {
	value []byte
	exp   time.Time
}

var now = func() time.Time {
	return time.Now()
}

type Memory struct {
	sessions map[string]item
	rw       sync.RWMutex
}

func New() *Memory {
	return &Memory{
		sessions: make(map[string]item),
	}
}

func (m *Memory) Store(_ context.Context, key, value []byte, exp time.Time) error {
	m.rw.Lock()
	m.sessions[string(key)] = item{
		value: value,
		exp:   exp,
	}
	m.rw.Unlock()
	return nil
}

func (m *Memory) Fetch(_ context.Context, key []byte) ([]byte, error) {
	m.rw.RLock()
	v, ok := m.sessions[string(key)]
	m.rw.RUnlock()
	if !ok || v.exp.Before(time.Now()) {
		return nil, errors.New("not found")
	}
	return v.value, nil
}

func (m *Memory) Delete(_ context.Context, key []byte) error {
	m.rw.Lock()
	delete(m.sessions, string(key))
	m.rw.Unlock()
	return nil
}
