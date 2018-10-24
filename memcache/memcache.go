package memcache_store

import (
	"context"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

type Store struct {
	mc *memcache.Client
}

func New(mc *memcache.Client) *Store {
	return &Store{mc: mc}
}

func (s *Store) Store(ctx context.Context, key, value []byte, exp time.Time) error {
	e := int32(exp.UTC().Unix())

	var err error
	done := make(chan struct{})
	go func() {
		err = s.mc.Set(&memcache.Item{
			Key:   string(key),
			Value: value,
			// 2038...
			Expiration: e,
		})
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
}

func (s *Store) Fetch(ctx context.Context, key []byte) ([]byte, error) {
	var i *memcache.Item
	var err error

	done := make(chan struct{})
	go func() {
		i, err = s.mc.Get(string(key))
		close(done)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}
	if err == memcache.ErrCacheMiss {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return i.Value, nil
}

func (s *Store) Delete(ctx context.Context, key []byte) error {
	var err error
	done := make(chan struct{})
	go func() {
		err = s.mc.Delete(string(key))
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
}
