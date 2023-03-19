package memcache_store

import (
	"context"
	"time"

	"github.com/abraithwaite/jeff/v2"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/vmihailenco/msgpack/v5"
)

// Store satisfies the jeff.Storage interface
type Store struct {
	mc *memcache.Client
}

// New initializes a new memcache Storage for jeff
func New(mc *memcache.Client) *Store {
	return &Store{mc: mc}
}

// Store satisfies the jeff.Store.Store method
func (s *Store) Store(ctx context.Context, key []byte, value []jeff.Session, exp time.Time) error {
	e := int32(exp.UTC().Unix())

	bts, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		err = s.mc.Set(&memcache.Item{
			Key:   string(key),
			Value: bts,
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

// Fetch satisfies the jeff.Store.Fetch method
func (s *Store) Fetch(ctx context.Context, key []byte) ([]jeff.Session, error) {
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

	var sl []jeff.Session
	err = msgpack.Unmarshal(i.Value, &sl)
	if err != nil {
		return nil, err
	}
	return sl, nil
}

// Delete satisfies the jeff.Store.Delete method
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
