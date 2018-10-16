package redis_store

import (
	"context"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
)

type Store struct {
	pool *redis.Pool
}

func New(p *redis.Pool) *Store {
	return &Store{pool: p}
}

var now = func() time.Time {
	return time.Now()
}

func (s *Store) Store(ctx context.Context, key, value []byte, exp time.Time) error {
	conn, err := s.pool.GetContext(ctx)
	defer conn.Close()
	if err != nil {
		return err
	}
	e := int(exp.Sub(now()) / time.Second)
	_, err = conn.Do("SETEX", key, strconv.Itoa(e), value)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) Fetch(ctx context.Context, key []byte) ([]byte, error) {
	conn, err := s.pool.GetContext(ctx)
	defer conn.Close()
	if err != nil {
		return nil, err
	}
	bs, err := redis.Bytes(conn.Do("GET", key))
	if err != nil && err != redis.ErrNil {
		return nil, err
	}
	return bs, nil
}

func (s *Store) Delete(ctx context.Context, key []byte) error {
	conn, err := s.pool.GetContext(ctx)
	defer conn.Close()
	if err != nil {
		return err
	}
	_, err = conn.Do("DEL", key)
	if err != nil && err != redis.ErrNil {
		return err
	}
	return nil
}
