//go:generate msgp
package jeff

import "time"

type Session struct {
	Exp   time.Time `msg:"exp"`
	Token []byte    `msg:"token"`
}

type SessionList []Session
