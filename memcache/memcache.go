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
	return s.mc.Set(&memcache.Item{
		Key:   string(key),
		Value: value,
		// 2038...
		Expiration: e,
	})
}

func (s *Store) Fetch(ctx context.Context, key []byte) ([]byte, error) {
	i, err := s.mc.Get(string(key))
	if err != nil {
		return nil, err
	}
	return i.Value, nil
}

func (s *Store) Delete(ctx context.Context, key []byte) error {
	return s.mc.Delete(string(key))
}
