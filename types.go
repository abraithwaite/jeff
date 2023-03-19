package jeff

import (
	"time"

	"golang.org/x/exp/slices"
)

type KeyValue struct {
	Key   string
	Value string
}

// Session represents the Session as it's stored in serialized form.  It's the
// object that gets returned to the caller when checking a session.
type Session struct {
	Key   []byte     `msgpack:"key"`
	Token []byte     `msgpack:"token"`
	Exp   time.Time  `msgpack:"exp"`
	Meta  []KeyValue `msgpack:"meta"`
}

func (s Session) Get(key string) (string, bool) {
	idx := slices.IndexFunc(s.Meta, func(kv KeyValue) bool {
		return kv.Key == key
	})
	if idx < 0 {
		return "", false
	}
	return s.Meta[idx].Value, true
}

// SessionList is a list of active sessions for a given key
type SessionList []Session
