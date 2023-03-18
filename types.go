package jeff

import (
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// Session represents the Session as it's stored in serialized form.  It's the
// object that gets returned to the caller when checking a session.
type Session struct {
	Key   []byte    `msgpack:"key"`
	Token []byte    `msgpack:"token"`
	Exp   time.Time `msgpack:"exp"`
	Meta  []byte    `msgpack:"meta"`
}

func (s *Session) Value(v any) error {
	return msgpack.Unmarshal(s.Meta, v)
}

// SessionList is a list of active sessions for a given key
type SessionList []Session
