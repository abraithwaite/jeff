package jeff

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// Storage provides the base level abstraction for implementing session
// storage.  Typically this would be memcache, redis or a database.
type Storage interface {
	// Store persists the session in the backend with the given expiration
	// Implementation must return value exactly as it is received.
	// Value will be given as...
	Store(ctx context.Context, key, value []byte, exp time.Time) error
	// Fetch retrieves the session from the backend.  If err != nil or
	// value == nil, then it's assumed that the session is invalid and Jeff
	// will redirect.  Expired sessions must return nil error and nil value.
	// Unknown (not found) sessions must return nil error and nil value.
	Fetch(ctx context.Context, key []byte) (value []byte, err error)
	// Delete removes the session given by key from the store. Errors are
	// bubbled up to the caller.  Delete should not return an error on expired
	// or missing keys.
	Delete(ctx context.Context, key []byte) error
}

func (j *Jeff) loadOne(ctx context.Context, key, tok []byte) (Session, error) {
	l, err := j.load(ctx, key)
	if err != nil {
		return Session{}, err
	}
	s, i := find(l, tok)
	if i < 0 {
		return Session{}, errors.New("session not found")
	}
	return s, nil
}

func (j *Jeff) load(ctx context.Context, key []byte) (SessionList, error) {
	stored, err := j.s.Fetch(ctx, key)
	if err != nil || stored == nil {
		return nil, err
	}
	var sl SessionList
	err = msgpack.Unmarshal(stored, &sl)
	return sl, err
}

func find(l SessionList, k []byte) (Session, int) {
	for i, s := range l {
		if subtle.ConstantTimeCompare(s.Token, k) == 1 {
			if s.Exp.Before(now()) {
				break
			}
			return s, i
		}
	}
	return Session{}, -1
}

func prune(l SessionList) SessionList {
	ret := make(SessionList, 0, len(l))
	for _, s := range l {
		if s.Exp.Before(now()) {
			continue
		}
		ret = append(ret, s)
	}
	return ret
}

func (j *Jeff) store(ctx context.Context, s Session) error {
	sl, err := j.load(ctx, s.Key)
	if err != nil {
		return err
	}
	if _, i := find(sl, s.Token); i >= 0 {
		sl[i] = s
	} else {
		sl = append(sl, s)
	}
	sl = prune(sl)
	bts, err := msgpack.Marshal(sl)
	if err != nil {
		return err
	}
	// Global Expiration 30d, TODO: make configurable
	return j.s.Store(ctx, s.Key, bts, now().Add(24*30*time.Hour))
}

// Clear deletes all sessions for a given key, or it deletes the selected
// sessions if a list of tokens is given.
func (j *Jeff) clear(ctx context.Context, key []byte, tokens ...[]byte) error {
	if len(tokens) == 0 {
		return j.s.Delete(ctx, key)
	}

	sl, err := j.load(ctx, key)
	if err != nil {
		return err
	}

	// if it's found, remove it.  This is O(N**2).  Not sure what the best way
	// to avoid this is.  Might want to impose limits on the number of sessions
	// per user and tokens passed into clear.
	for _, tok := range tokens {
		if _, i := find(sl, tok); i >= 0 {
			sl = append(sl[:i], sl[i+1:]...)
		}
	}

	// prune expired sessions
	sl = prune(sl)
	bts, err := msgpack.Marshal(sl)
	if err != nil {
		return err
	}
	// Global Expiration 30d, TODO: make configurable
	return j.s.Store(ctx, key, bts, now().Add(24*30*time.Hour))
}
