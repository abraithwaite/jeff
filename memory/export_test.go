package memory

import "time"

func SetTime(f func() time.Time) {
	now = f
}
