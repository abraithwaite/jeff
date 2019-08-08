package redis_store

import (
	"context"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
)

// Store satisfies the jeff.Storage interface
type Store struct {
	pool *redis.Pool
}

var now = func() time.Time {
	return time.Now()
}

// New initializes a new redis Storage for jeff
func New(p *redis.Pool) *Store {
	return &Store{pool: p}
}

// Store satisfies the jeff.Store.Store method
func (s *Store) Store(ctx context.Context, key, value []byte, exp time.Time) error {
	conn, err := s.pool.GetContext(ctx)
	defer conn.Close()
	if err != nil {
		return err
	}
	e := int(exp.Sub(now()) / time.Second)

	done := make(chan struct{})
	go func() {
		_, err = conn.Do("SETEX", key, strconv.Itoa(e), value)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
}

// Fetch satisfies the jeff.Store.Fetch method
func (s *Store) Fetch(ctx context.Context, key []byte) ([]byte, error) {
	conn, err := s.pool.GetContext(ctx)
	defer conn.Close()
	if err != nil {
		return nil, err
	}

	var bs []byte
	done := make(chan struct{})
	go func() {
		bs, err = redis.Bytes(conn.Do("GET", key))
		close(done)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		if err != nil && err != redis.ErrNil {
			return nil, err
		}
		return bs, nil
	}
}

// Delete satisfies the jeff.Store.Delete method
func (s *Store) Delete(ctx context.Context, key []byte) error {
	conn, err := s.pool.GetContext(ctx)
	defer conn.Close()
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		_, err = conn.Do("DEL", key)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		if err != nil && err != redis.ErrNil {
			return err
		}
		return nil
	}
}
