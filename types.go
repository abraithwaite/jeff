//go:generate msgp
package jeff

import "time"

type Session struct {
	Key   []byte    `msg:"key"`
	Token []byte    `msg:"token"`
	Meta  []byte    `msg:"meta"`
	Exp   time.Time `msg:"exp"`
}

type SessionList []Session
