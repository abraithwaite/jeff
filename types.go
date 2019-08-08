//go:generate msgp
package jeff

import "time"

// Session represents the Session as it's stored in serialized form.  It's the
// object that gets returned to the caller when checking a session.
type Session struct {
	Key   []byte    `msg:"key"`
	Token []byte    `msg:"token"`
	Meta  []byte    `msg:"meta"`
	Exp   time.Time `msg:"exp"`
}

// SessionList is a list of active sessions for a given key
type SessionList []Session
