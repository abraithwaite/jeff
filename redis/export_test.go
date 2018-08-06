package redis_store

import "time"

func SetTime(f func() time.Time) {
	now = f
}
